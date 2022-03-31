sip协议 RFC3261

### SIP规定了六种方法
INVITE      用于邀请用户或服务参加一个会话
ACK         请求用于客户端向服务器证实收到对INVITE请求的最终应答
CANCEL      用于取消一个Call-ID From To Cseq 字段相同正在进行的请求,但取消不了已完成的请求
OPTIONS     用于向服务器查询其能力
BYE         用于结束会话
REGISTER    用于客户向注册服务器注册用户位置等信息

### SIP消息头字段
via          给出请求消息迄今为止经过的路径
Request-URI  注册请求的目的地址
Max-Forwords 请求消息允许被转发的次数
From         请求的发起者
To           请求的目的接收方
Call-ID      唯一标识特定邀请或某个客户机的注册请求消息
Cseq         标识服务器发出的不同请求,若Call-ID相同Cseq值必须各不相同
Contact      给出一个URL,用户可以根据此URL进一步的通讯
Content-Length   消息体的大小
Content-Type     消息体的媒体类型
Expires      消息内容截止的日期和时间
User-Agent   发起请求的用户代理客户及相关的信息
### SIP消息体
v       协议的版本
o       与会话所有者的相关参数
s       会话标题或会话名称
c       真正流媒体使用的IP地址
t       会话的开始时间与结束时间
m       会话所支持的媒体类型
a       媒体的属性行
### SIP状态码定义
1XX     请求已经收到继续处理请求
2XX     行动已成功的接收到
3XX     为完成呼叫请求还需采取进一步动作
4XX     请求有语法错误不能被服务器端执行,客户端需修改请求,再次重发
5XX     服务器出错不能执行合法请求
6XX     任何服务器都不能执行请求

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
