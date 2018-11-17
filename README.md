# ADLand

ADLand 是一个根据访客动态返回页面的Webserver，由go语言编写, ADLand使用Aho–Corasick多关键字匹配算法来高速过滤访客信息，同时借助go达到较好的页面分发性能

# Features
- 选择以http或https方式运行，当以https运行时，自动从Let's Encrypt获取免费https证书
- 支持按照任意数量的IP地址段过滤，可以是A,B,C类地址或完全ip地址
- 支持按照纯真IP数据库的任意地理位置过滤
- 支持按User-agent的任意数量的字符串过滤
- 支持按同一Cookie的访问次数过滤

# 目录和文件

1. adland.exe    主程序
2. routes.txt     配置文件，下一节详细介绍
3. libs\qqwry.dat      IP地址库(2018.9版)，此库用于过滤文字性的IP地址位置（例如“北京”，“上海”， “阿里云”），如果此库IP信息存在不准或不全没有关系，配置文件中的blocked_ips可以配置任意数目的任意ip段， qqwry.dat在运行时会完全加载到内存中以方便高速查询
4. libs\cookie.db      记录Cookie访问次数的数据库，用于按照同一Cookie访问次数过滤，该数据库使用leveldb以提高性能
5. templates\.     需要动态返回的页面模版都放在这个目录
6. static\      动态页面中引用的其他静态页面元素(例如css,js,图片等)放在这个目录，当webserver的url以 /s/ 开头时，则在此静态目录下直接调用文件，例如 http://server/s/a.css 即返回 static\a.css

# routes.txt 配置文件介绍

例子：
```
[location]
url=/ad1.html
if_allowed=a.html
if_blocked=b.html
blocked_ips=127.0,111.104,23.24.25
blocked_areas=北京,天津
blocked_cookies=3
allowed_uas=Chrome,Wechat
```

routes.txt可以由多个 [location] 开始的数行配置构成， 每一行格式为key=value，每一节[location]代表了一个动态分配页面 ，对于上面例子配置中的每行参数解释如下 ：

url=/ad1.html            必填，访客看到的url，例如 http://server/ad1.html，url可以写成任意形式，例如url=/hello/world/foolish?yes ， 实际返回的页面是由if_allowed和if_blocked定义
if_allowed=a.html     必填， 当访客被允许访问时，返回 templates\a.html
if_blocked=b.html     必填， 当访客被禁止访问时，返回 templates\b.html
blocked_ips=127.0,111.104,23.24.25 可选，当blocked_ips=后面为空时不进行ip地址过滤，否则就是过滤多个IP段（以逗号,分割），只要访客IP匹配其中任意一段，访问则导向if_blocked定义的页面
blocked_areas=北京,天津  可选，当blocked_areas=后面为空时不进行ip区域过滤, 否则就是过滤多个IP所在位置（以逗号,分割），只要访客IP所在位置信息匹配其中任意一段，访问则导向if_blocked定义的页面
blocked_cookies=3 可选，如果不为空，那么当同一访客访问次数超过blocked_cookies时，访问则导向if_blocked定义的页面
allowed_uas=Chrome,Wechat  可选，如果不为空，那么只有匹配allowed_uas中的访客才能被导向if_allowed定义的页面，否则就是if_blocked

当访客没有被以上的各种过滤器block掉时，访问就会被导向if_allowed定义的页面

# ADLand 运行参数
adland -h
adland -p 80  以指定端口运行
adland -s 以https方式运行，此时-d参数必须
adland -d xxx.com 指定域名 （用于种植cookie时使用)
adland -r xxx.txt 指定配置文件 （默认为 routes.txt )

ADLand运行时会打印访客日志，格式为
时间， IP， IP区域信息， Cookie的唯一标识，User-Agent， 是否被allow或者block ( blocked就会导向if_blocked定义页面，否则就是if_allowed定义页面）






