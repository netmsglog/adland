package main

import (
	"crypto/tls"
	"encoding/binary"
	"flag"
	"html/template"
	"io/ioutil"
	"log"
	"mutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nu7hatch/gouuid"
	"github.com/syndtr/goleveldb/leveldb"
	"golang.org/x/crypto/acme/autocert"
)

var (
	optPort   string
	optDomain string
	optRoutes string
	optSecure bool
	cachedb   *leveldb.DB
)

// Route - my route of url match
type Route struct {
	ifAllowed      *template.Template
	ifBlocked      *template.Template
	blockedIps     *mutil.Matcher // aho ip matcher
	blockedAreas   *mutil.Matcher // aho area matcher
	blockedCookies int            // how many repeated visting times
	allowedUas     *mutil.Matcher // user-agent
}

// Router - my router struct
type Router struct {
	routes map[string]*Route
}

// UUID - get a unique cookie string
func UUID() string {
	uu, _ := uuid.NewV4()
	return uu.String()
}

// Trim - util
func Trim(str string) string {
	return strings.Trim(str, "\r\n\t ")
}

// Left - util
func Left(str string, sep string) string {
	pos := strings.Index(str, sep)
	if pos == -1 {
		return str
	}
	return str[:pos]
}

func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func btoi(b []byte) int {
	return int(binary.BigEndian.Uint64(b))
}

func cacheGet(url string) int {
	var v []byte
	var rt int
	var err error

	v, err = cachedb.Get([]byte(url), nil)
	if v != nil {
		t := btoi(v)
		rt = t + 1
	} else {
		rt = 1
	}
	err = cachedb.Put([]byte(url), itob(rt), nil)
	if err != nil {
		log.Println(err)
	}
	return rt
}

// NewRouter - create a new router
func NewRouter() *Router {
	return &Router{routes: make(map[string]*Route)}
}

// INT - convert string to int
func INT(str string) int {
	i, _ := strconv.Atoi(str)
	return i
}

func str2arr(str string) []string {
	words := make([]string, 0)
	segs := strings.Split(str, ",")
	for _, seg := range segs {
		s := Trim(seg)
		if s != "" {
			words = append(words, s)
		}
	}
	return words
}

// NewRoute - create a route
func NewRoute(ifallowed string, ifblocked string, blockedips string, blockedareas string, blockedcookies string, alloweduas string) *Route {
	rt := Route{}
	rt.ifAllowed = template.Must(template.ParseFiles(filepath.FromSlash("./template/" + ifallowed)))
	rt.ifBlocked = template.Must(template.ParseFiles(filepath.FromSlash("./template/" + ifblocked)))

	if blockedips != "" {
		rt.blockedIps = mutil.NewStringMatcher(str2arr(blockedips))
	} else {
		rt.blockedIps = nil
	}
	if blockedareas != "" {
		rt.blockedAreas = mutil.NewStringMatcher(str2arr(blockedareas))
	} else {
		rt.blockedAreas = nil
	}
	if blockedcookies != "" {
		rt.blockedCookies = INT(blockedcookies)
	} else {
		rt.blockedCookies = -1
	}
	if alloweduas != "" {
		rt.allowedUas = mutil.NewStringMatcher(str2arr(alloweduas))
	} else {
		rt.allowedUas = nil
	}
	return &rt
}

func (rt *Route) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var uaid string
	var blocked bool

	agent := req.Header.Get("User-Agent")

	ucookie, err := req.Cookie("uaid")
	if err != nil {
		uaid = UUID()
		w.Header().Set("P3P", "CP=\"CAO DSP COR CUR ADM DEV TAI PSA PSD IVAi IVDi CONi TELo OTPi OUR DELi SAMi OTRi UNRi PUBi IND PHY ONL UNI PUR FIN COM NAV INT DEM CNT STA POL HEA PRE GOV\"")
		ck := http.Cookie{Name: "uaid", Value: uaid, Path: "/", Domain: optDomain, Expires: time.Now().Add(time.Hour * 87600)}
		w.Header().Set("Set-Cookie", ck.String())
	} else {
		uaid = ucookie.Value
	}

	rip := Left(req.RemoteAddr, ":")
	co, ci := mutil.LookupIP(rip)

	if rt.blockedIps != nil {
		blocked = rt.blockedIps.Match(rip)
		if blocked {
			log.Println(rip, co, ci, uaid, agent, " is blocked by blocked_ips")
			rt.ifBlocked.Execute(w, nil)
			return
		}
	}
	if rt.blockedAreas != nil {

		blocked = rt.blockedAreas.Match(co) || rt.blockedAreas.Match(ci)
		if blocked {
			log.Println(rip, co, ci, uaid, agent, " is blocked by blocked_areas")
			rt.ifBlocked.Execute(w, nil)
			return
		}
	}
	cnt := cacheGet(uaid)
	if (rt.blockedCookies > 0) && (cnt > rt.blockedCookies) {
		log.Println(rip, co, ci, uaid, agent, " is blocked by blocked_cookies ", cnt)
		rt.ifBlocked.Execute(w, nil)
		return
	}
	if rt.allowedUas != nil {
		blocked = !rt.allowedUas.Match(agent)
		if blocked {
			log.Println(rip, co, ci, uaid, agent, " is blocked by allowed_uas")
			rt.ifBlocked.Execute(w, nil)
			return
		}
	}
	log.Println(rip, co, ci, uaid, agent, " get accessed")
	rt.ifAllowed.Execute(w, nil)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	// static files
	if strings.HasPrefix(path, "/s/") {
		http.StripPrefix("/s/", http.FileServer(http.Dir("./static/"))).ServeHTTP(w, req)
		return
	}
	route, ok := r.routes[path]
	if ok {
		route.ServeHTTP(w, req)
	} else {
		http.NotFound(w, req)
	}
}

func postfix(str string, prefix string) string {
	return strings.Trim(strings.TrimPrefix(str, prefix), "\r\n\t ")
}

func (r *Router) loadRoutes(fn string) {
	var url string
	var ifallowed string
	var ifblocked string
	var blockedips string
	var blockedareas string
	var blockedcookies string
	var alloweduas string
	var inlocation bool

	f, err := ioutil.ReadFile(fn)
	if err != nil {
		log.Fatal(err)
	}
	inlocation = false
	lines := strings.Split(string(f), "\n")

	for lineno, line := range lines {
		l := Trim(line)
		if l == "[location]" || lineno == len(lines)-1 {
			if inlocation { // terminate last location
				if url == "" || ifallowed == "" || ifblocked == "" {
					log.Fatalln("Missing url or if_allowed or if_blocked in config near ", lineno+1)
				}
				log.Println("Add new route [url=", url, "][ifblocked=", ifblocked, "][ifallowed=",
					ifallowed, "][blockedips=", blockedips, "][blockedaeas=", blockedareas, "][blockedcookies=", blockedcookies, "][allowed_uas=", alloweduas, "]")
				rt := NewRoute(ifallowed, ifblocked, blockedips, blockedareas, blockedcookies, alloweduas)
				r.routes[url] = rt
			} else { // start a new location route
				inlocation = true
				url = ""
				ifallowed = ""
				ifblocked = ""
				blockedips = ""
				blockedareas = ""
				blockedcookies = ""
				alloweduas = ""
			}
		} else if strings.HasPrefix(l, "url=") {
			url = postfix(l, "url=")
		} else if strings.HasPrefix(l, "if_allowed=") {
			ifallowed = postfix(l, "if_allowed=")
		} else if strings.HasPrefix(l, "if_blocked=") {
			ifblocked = postfix(l, "if_blocked=")
		} else if strings.HasPrefix(l, "blocked_ips=") {
			blockedips = postfix(l, "blocked_ips=")
		} else if strings.HasPrefix(l, "blocked_areas=") {
			blockedareas = postfix(l, "blocked_areas=")
		} else if strings.HasPrefix(l, "blocked_cookies=") {
			blockedcookies = postfix(l, "blocked_cookies=")
		} else if strings.HasPrefix(l, "allowed_uas=") {
			alloweduas = postfix(l, "allowed_uas=")
		}
	}

}

func main() {
	flag.StringVar(&optPort, "p", "80", "listen port")
	flag.StringVar(&optDomain, "d", "", "cookie domain")
	flag.BoolVar(&optSecure, "s", false, "run in secure mode (https)")
	flag.StringVar(&optRoutes, "r", "routes.txt", "routes config file")

	flag.Parse()

	if optSecure {
		if optDomain == "" {
			log.Fatalln("You must specify the domain (xxx.com) using -d")
		}
	}

	log.Println("Loading qqwry.dat...")
	err := mutil.LoadIPLoc(filepath.FromSlash("./lib/qqwry.dat"))
	if err != nil {
		log.Fatalln(err)
	}

	log.Println("Initializing cookie history db...")
	cachedb, err = leveldb.OpenFile(filepath.FromSlash("./lib/cookie.db"), nil)
	if err != nil {
		log.Fatalln(err)
	}
	defer cachedb.Close()

	log.Println("Loading route configuration...")
	r := NewRouter()
	r.loadRoutes(optRoutes)

	if optSecure {
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(optDomain, "www."+optDomain),
			Cache:      autocert.DirCache("lib"),
		}
		go func() {
			log.Fatal(http.ListenAndServe(":http",
				m.HTTPHandler(nil)))
		}()
		s := &http.Server{
			Addr:           ":https",
			Handler:        r,
			ReadTimeout:    20 * time.Second,
			WriteTimeout:   20 * time.Second,
			MaxHeaderBytes: 1 << 20,
			TLSConfig: &tls.Config{
				GetCertificate: m.GetCertificate,
			},
		}
		log.Println("ADLand server started at 443(https) domain", optDomain, " ...")
		err = s.ListenAndServeTLS("", "")
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		s := &http.Server{
			Addr:           ":" + optPort,
			Handler:        r,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
		}
		log.Println("ADLand server started at", optPort, ", domain", optDomain, " ...")
		err = s.ListenAndServe()
		if err != nil {
			log.Fatalln(err)
		}
	}
}
