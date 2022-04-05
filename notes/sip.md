SIP（Session initialization Protocol，会话初始协议）是由IETF制定的多媒体通信协议
是一个基于文本的应用层控制协议，用于创建、修改和释放一个或多个参与者的会话。
SIP 是一种源于互联网的IP 语音会话控制协议，具有灵活、易于实现、便于扩展等特点。
SIP 出现于二十世纪九十年代中期，源于哥伦比亚大学计算机系副教授 Schulzrinne 及其研究小组的研究。
Schulzrinne 教授除与人共同提出通过 Internet 传输实时数据的实时传输协议(RTP)
SIP 规定一个或多个参与方的终端设备如何能够建立、修改和中断连接，而不论是语音、视频、数据或基于 Web 的内容。
SIP 的一个重要特点是它不定义要建立的会话的类型，而只定义应该如何管理会话。
SIP可以用于众多应用和服务中，包括交互式游戏、音乐和视频点播以及语音、视频和 Web 会议。
SIP 能够连接使用任何 IP 网络（有线 LAN 和 WAN、公共 Internet 骨干网、移动 2.5G、3G 和 Wi-Fi）和任何 IP 设备（电话、PC、PDA、移动手持设备）的用户
SIP 的根本价值在于它能够将很多功能(协议)组合起来，形成各种更大规模的无缝通信服务。

https://baike.baidu.com/item/SIP/33921
SIP 会话使用多达四个主要组件:
SIP 用户代理、SIP 注册服务器、SIP 代理服务器和 SIP 重定向服务器

SIP协议可以和若干个其他协议进行协作
与负责语音质量的资源预留协议(RSVP)
与负责定位的轻型目录访问协议(LDAP)
与负责实时传输协议(RTP)
为了描述消息内容的负载情况和特点，SIP 使用 会话描述协议 (SDP) 来描述终端设备的特点

GBT-28181_2011.pdf
GBT-28181_2016.pdf

第一个 SIP 规范，即 RFC 2543
2001 年发布了 SIP 规范 RFC 3261
RFC 3262 对临时响应的可靠性作了规定
RFC 3263 确立了 SIP代理服务器的定位规则
RFC 3264 提供了提议/应答模型
RFC 3265 确定了具体的事件通知

Register    Rfc3261
Invite      Rfc3261
Info        Rfc2976
Message     Rfc3428

SIP协议的亮点却不在于它的强大，而是在于：简单！
SIP协议是一个Client/Sever协议，因此SIP消息分两种：请求消息和响应消息。
请求消息是SIP客户端为了激活特定操作而发给服务器端的消息。
响应消息用于对请求消息进行响应，指示呼叫的成功或失败状态。

### SIP常用的方法
INVITE      用于邀请用户或服务参加一个会话
ACK         请求用于客户端向服务器证实收到对INVITE请求的最终应答
PRACK       表示对1xx响应消息的确认请求消息。
BYE         用于结束会话
CANCEL      用于取消一个Call-ID From To Cseq 字段相同正在进行的请求,但取消不了已完成的请求
REGISTER    用于客户向注册服务器注册用户位置等信息
OPTIONS     用于向服务器查询其能力
INFO        
NOTIFY
REFER
SUBSCRIBE
UPDATE

终端向代理服务器发送register消息注销，其头中expire字段设置为0

### SIP消息头字段
Request-URI  REGISTER sip:11000000122000000034@1100000012 SIP/2.0 注册请求的目的地址
Via          给出请求消息迄今为止经过的路径, Via头域就是用来指示如何将响应沿原路返回到UA的
From         发起请求方的地址。一般采用USERINFO@HOSTPORT形式。该域同时带有一个TAG参数，是随机产生的整数。
To           请求的目的接收方
Call-ID      用于识别呼叫参数，在同一个DIALOG中，该参数不发生变化。
             该参数与FROM中的TAG参数、TO域中的TAG参数相结合用以保证呼叫的惟一性。
Cseq         Command Sequence Number 用于标识事务并对事务排序
             CSeq能够区分某个请求是新请求还是重发的请求
Contact      是UA希望用来接收请求的地址，后续请求可以用它来联系到当前UA
Allow        这个字段是使用逗号分割的一个list，指明了调用者支持或使用的SIP方法
Max-Forwords 请求消息允许被转发的次数
User-Agent   发起请求的用户代理客户及相关的信息
Expires      给出消息内容超期的时间
Record-Route 由于CONTACT域的存在使得两个用户后续的请求消息可能不经过PROXY,
             为了运营需要，PROXY在初始INVITE消息中增加了RECORD-ROUTE域，
             这样可以保证后续请求（例如BYE消息）经过PROXY
Require      UAC通过Require字段列出的选项标签，告知UAS处理请求时需要支持的选项，本字段为可选
Content-Type     消息体的媒体类型
Content-Length   消息体的大小

### SIP消息头字段说明
https://www.cnblogs.com/zhangming-blog/articles/5900244.html
From Tag, To Tag 和 Call-ID 构成了dialog信息，可以唯一标识一个dialog
在本次呼叫(Call)中的所有请求和响应将使用同样dialog信息

### 参考文章
GB28181报文详解 (有过程说明 必看)
https://zhuanlan.zhihu.com/p/98533891
gb28181抓包
https://blog.csdn.net/weixin_43360707/article/details/120975297
一次完整的通话过程SIP报文分析
https://blog.51cto.com/u_15054050/3833211

### SIP消息体
v       Version Number 协议的版本
o       Origin 与会话所有者的相关参数
s       Subject 会话标题或会话名称
c       Connection Data 连接信息, 真正流媒体使用的IP地址
t       Time 会话活动时间, 会话的开始时间与结束时间
m       Media(type, port, RTP/AVP Profile), 媒体名称和传输地址
a       媒体的属性行

常用的一些响应消息：
### SIP状态码定义
1XX     请求已经收到继续处理请求
2XX     行动已成功的接收到
3XX     为完成呼叫请求还需采取进一步动作
4XX     请求有语法错误不能被服务器端执行,客户端需修改请求,再次重发
5XX     服务器出错不能执行合法请求
6XX     任何服务器都不能执行请求

100     试呼叫（Trying）
180     振铃（Ringing）
181     呼叫正在前转（Call is Being Forwarded）
200     成功响应（OK）
302     临时迁移（Moved Temporarily）
400     错误请求（Bad Request）
401     未授权（Unauthorized）
403     禁止（Forbidden）
404     用户不存在（Not Found）
408     请求超时（Request Timeout）
480     暂时无人接听（Temporarily Unavailable）
486     线路忙（Busy Here）
504     服务器超时（Server Time-out）
600     全忙（Busy Everywhere）

### sip业务交互流程
注册流程
注销流程
会话建立流程
会话断开流程

#### 注册流程
1.client向server发送注册请求
2.server返回401, 并带上密钥
3.server收到401和密钥, 通过密钥加密注册密码, 然后返回给server
4.server返回200 验证成功

### sipClient REGISTER
摄像头配置的注册间隔是60秒(最小), 服务端收到的注册间隔时间是 95秒

2022/03/30 15:02:20 sip.go:31: ---------->> new tcp(sip) connect
2022/03/30 15:02:20 sip.go:32: RemoteAddr: 10.3.220.151:50287
2022/03/30 15:02:20 sip.go:15: len: 394, data: REGISTER sip:11000000122000000034@1100000012 SIP/2.0
Via: SIP/2.0/TCP 10.3.220.151:50287;rport;branch=z9hG4bK1331290990
From: <sip:11010000121310000034@1100000012>;tag=783666626
To: <sip:11010000121310000034@1100000012>
Call-ID: 543893519
CSeq: 1 REGISTER
Contact: <sip:11010000121310000034@10.3.220.151:5060>
Max-Forwards: 70
User-Agent: IP Camera
Expires: 3600
Content-Length: 0


2022/03/30 15:03:55 sip.go:31: ---------->> new tcp(sip) connect
2022/03/30 15:03:55 sip.go:32: RemoteAddr: 10.3.220.151:38531
2022/03/30 15:03:55 sip.go:15: len: 396, data: REGISTER sip:11000000122000000034@1100000012 SIP/2.0
Via: SIP/2.0/TCP 10.3.220.151:38531;rport;branch=z9hG4bK1369242141
From: <sip:11010000121310000034@1100000012>;tag=1848982013
To: <sip:11010000121310000034@1100000012>
Call-ID: 1034572694
CSeq: 1 REGISTER
Contact: <sip:11010000121310000034@10.3.220.151:5060>
Max-Forwards: 70
User-Agent: IP Camera
Expires: 3600
Content-Length: 0


2022/03/30 15:05:30 sip.go:31: ---------->> new tcp(sip) connect
2022/03/30 15:05:30 sip.go:32: RemoteAddr: 10.3.220.151:51103
2022/03/30 15:05:30 sip.go:15: len: 394, data: REGISTER sip:11000000122000000034@1100000012 SIP/2.0
Via: SIP/2.0/TCP 10.3.220.151:51103;rport;branch=z9hG4bK107423043
From: <sip:11010000121310000034@1100000012>;tag=1168205899
To: <sip:11010000121310000034@1100000012>
Call-ID: 104027563
CSeq: 1 REGISTER
Contact: <sip:11010000121310000034@10.3.220.151:5060>
Max-Forwards: 70
User-Agent: IP Camera
Expires: 3600
Content-Length: 0
