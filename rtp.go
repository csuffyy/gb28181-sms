package main

import "log"

// PlayloadType   uint8  // 7bit, 有效荷载类型
// Rtp 可以装载 h264数据 也可以装载 ps数据
// RTP_PAYLOAD_TYPE_PCMU    = 0, // g711u
// RTP_PAYLOAD_TYPE_PCMA    = 8, // g711a
// RTP_PAYLOAD_TYPE_JPEG    = 26,
// RTP_PAYLOAD_TYPE_H264    = 96,
// RTP_PAYLOAD_TYPE_H265    = 97,
// RTP_PAYLOAD_TYPE_OPUS    = 98,
// RTP_PAYLOAD_TYPE_AAC     = 99,
// RTP_PAYLOAD_TYPE_G726    = 100,
// RTP_PAYLOAD_TYPE_G726_16 = 101,
// RTP_PAYLOAD_TYPE_G726_24 = 102,
// RTP_PAYLOAD_TYPE_G726_32 = 103,
// RTP_PAYLOAD_TYPE_G726_40 = 104,
// RTP_PAYLOAD_TYPE_SPEEX   = 105,
// 数据太长的话 就要拆分 放到多个Rtp包中
// 1 + 1 + 2 + 4 + 4 + 4*n = 16byte
type RtpHeader struct {
	Version        uint8    // 2bit, V, rtp协议版本 固定值 0x2
	Padding        uint8    // 1bit, P, rtp包尾部是否有填充字节, 填充比特不算作负载的一部分, 填充的最后一个字节指明可以忽略多少个填充比特。填充可能用于某些具有固定长度的加密算法，或者用于在底层数据单元中传输多个 RTP 包。
	Extension      uint8    // 1bit, X, rtp固定头后面是否有扩展头
	CsrcCount      uint8    // 4bit, CC, csrc数目 最多16条
	Marker         uint8    // 1bit, M, 不同的有效载荷有不同的含义, 对于视频, 标记一帧的结束; 对于音频，标记会话的开始;
	PlayloadType   uint8    // 7bit, PT, 有效荷载类型, 用于说明RTP报文中有效载荷的类型, 在流媒体中是用来区分音频流和视频流的
	SequenceNumber uint16   // 16bit, SN, 用于标识发送者所发送的RTP报文的序列号，每发送一个报文，序列号增1. 这个字段当下层的承载协议用UDP的时候，网络状况不好的时候可以用来检查丢包。同时出现网络抖动的情况可以用来对数据进行重新排序，序列号的初始值是随机的，同时音频包和视频包的sequence是分别记数的。序列号的初始值是随机的。
	Timestamp      uint32   // 32bit, TS, 必须使用90 kHz 时钟频率, 记录了该包中数据的第一个字节的采样时刻。在一次会话开始时，时间戳初始化成一个初始值。即使在没有信号发送时，时间戳的数值也要随时间而不断地增加(时间在流逝嘛)
	Ssrc           uint32   // 32bit, 同步信源 同步源就是指RTP包流的来源, 在同一个RTP会话中不能有两个相同的SSRC值。该标识符是随机选取的 RFC1889推荐了MD5随机算法。
	Csrc           []uint32 // 32bit, 贡献(特约)信源 用来标志对一个RTP混合器产生的新包有贡献的所有RTP包的源。由混合器将这些有贡献的SSRC标识符插入表中。SSRC标识符都被列出来，以便接收端能正确指出交谈双方的身份。
}

//计算音频打包发送间隔、打包字节数???
// 音频的帧率	fps = 20
// 采样率		sample_rate = 8000 Hz
// 码率			bitrate = 64000	bps
// 打包发送间隔	send_interval = 1 / 20 = 0.05s = 50000us
// 每包数据长度 audio_need_len = 64000 bps * 0.05s = 3200 bit = 400 bytes
// udp数据长度	data_len = 1500 - 20(ip头) - 8(udp头) - 12(rtp头)

//RTP协议
// https://www.cnblogs.com/abelchao/articles/11661706.html

// rtp包大小限制
// rtp使用tcp协议发送数据的时候, rtp包大小 不受限制, 因为tcp提供分组到达的检测
// rtp使用udp协议发送数据的时候, rtp包大小 不能大于mtu(一般是1500字节)
// ip包头20字节, udp包头8字节, rtp包头12字节, 所以rtp负载 1500 - 20 - 8 - 12 = 1460字节
// 因为音频编码数据的一帧通常是小于MTU的，所以通常是直接使用RTP协议进行封装和发送。
// 如果负载数据长度大于 1460 字节, 由于我们没有在应用层分割数据，将会产生大于 MTU 的rtp包
// 在 IP 层其将会被分割成几个小于 MTU 尺寸的包, 因为 IP 和 UDP 协议都 没有提供分组到达的检测,
// 分割后就算所有包都成功接收, 但是 由于只有第 一个包中包含有完 整的 RTP 头信息，
// 而 RTP 头中没有关于载荷长度的标识, 因此判断 不出该 RTP 包是否有分割丢失，只能认为完整的接收了

// MTU是什么
// MTU(Maximum Transmission Unit)是网络最大传输单元, 是指网络通信协议的某一协议层上 所能通过的数据包最大字节数.
// 通信主机A  网络设备(路由器/交换机) 通信主机B, 这些设备上 都有MTU的设置，一般都是1500
// 当MTU不合理时会造成如下问题
// 1 本地MTU值大于网络MTU值时，本地传输的"数据包"过大导致网络会拆包后传输，不但产生额外的数据包，而且消耗了“拆包、组包”的时间。
// 2 本地MTU值小于网络MTU值时，本地传输的数据包可以直接传输，但是未能完全利用网络给予的数据包传输尺寸的上限值，传输能力未完全发挥。
// 什么是合理的MTU值?
// 所谓合理的设置MTU值，就是让本地的MTU值与网络的MTU值一致，既能完整发挥传输性能，又不让数据包拆分。
// 怎么探测合理的MTU? 发送大小是1460(+28)字节的包, 20字节的ip头，和8字节的icmp封装
// linux探测MTU值
// [localhost:~]# ping -s 1460 -M do baidu.com  小于等于网络mtu值, 都会返回正常
// PING baidu.com (220.181.38.251) 1460(1488) bytes of data.
// 1468 bytes from 220.181.38.251 (220.181.38.251): icmp_seq=1 ttl=47 time=4.39 ms
// [localhost:~]# ping -s 1500 -M do baidu.com  大于网络mtu值, 会返回错误信息
// PING baidu.com (220.181.38.251) 1500(1528) bytes of data.
// ping: local error: Message too long, mtu=1500
// linux临时修改MTU值
// ifconfig eth0 mtu 1488 up

// rtp视频数据的分包
// 如果 Nalu的SIZE 小于 MTU(网络最大传输单元, 一般是1500byte)
// 此时 单个NALU 放入 RTP payload中
// H.264 NALU格式: [Start Code] [NALU Header] [NALU Payload]
// 其中[Start Code] 在打包进Rtp Playload 的时候要去除
// 当NALU 放在网络中传输,即加入NALU header 的时候,会去掉0x00000001开始码, 因为有header 来区分, 但是存放磁盘的时候, 媒体文件会加开始码

/*
// 单一NAL单元模式 结构
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|V=2|P|X|  CC   |M|     PT      |       sequence number         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                           timestamp                           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           synchronization source (SSRC) identifier            |
+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+
|            contributing source (CSRC) identifiers             |
|                             ....                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|F|NRI|  type   |                                               |
+-+-+-+-+-+-+-+-+                                               |
|                                                               |
|               Bytes 2..n of a Single NAL unit                 |
|                                                               |
|                               +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                               :...OPTIONAL RTP padding        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

// NALU 因为过大，按照 FU-A分片的流程
// 1）第一个FU-A包的FU indicator：F应该为当前NALU头的F，而NRI应该为当前NALU头的NRI，Type则等于28，表明它是FU-A包。FU header生成方法：S = 1，E = 0，R = 0，Type则等于NALU头中的Type。
// 2）后续的N个FU-A包的FU indicator和第一个是完全一样的，如果不是最后一个包，则FU header应该为：S = 0，E = 0，R = 0，Type等于NALU头中的Type。
// 3）最后一个FU-A包FU header应该为：S = 0，E = 1，R = 0，Type等于NALU头中的Type。

/*
// FU-A结构
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|V=2|P|X|  CC   |M|     PT      |       sequence number         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                           timestamp                           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           synchronization source (SSRC) identifier            |
+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+
|            contributing source (CSRC) identifiers             |
|                             ....                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| FU indicator  |   FU header   |                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               |
|                                                               |
|                         FU payload                            |
|                                                               |
|                               +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                               :...OPTIONAL RTP padding        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

/*
FU indicator 就是
+---------------+
|0|1|2|3|4|5|6|7|
+-+-+-+-+-+-+-+-+
|F|NRI|  Type   |
+---------------+

FU header 就是
+---------------+
|0|1|2|3|4|5|6|7|
+-+-+-+-+-+-+-+-+
|S|E|R|  Type   |
+---------------+
*/

// SSRC 和 CSRC 的说明
//这里的同步信源是指产生媒体流的信源，它通过RTP报头中的一个32位数字SSRC标识符来标识，而不依赖于网络地址，接收者将根据SSRC标识符来区分不同的信源，进行RTP报文的分组。特约信源是指当混合器接收到一个或多个同步信源的RTP报文后，经过混合处理产生一个新的组合RTP报文，并把混合器作为组合RTP报文的 SSRC，而将原来所有的SSRC都作为CSRC传送给接收者，使接收者知道组成组合报文的各个SSRC。
// 考虑到在Internet这种复杂的环境中举行视频会议，RTP定义了两种中间系统: 混合器(Mixer) 和 转换器(Translator)。
// 混合器(Mixer)
// 在Internet上举行视频会议时，可能有少数参加者通过低速链路与使用高速网络的多数参加者相连接。为了不强制所有会议参加者都使用低带宽和低质量的数据编码，RTP允许在低带宽区域附近使用混合器作为RTP级中继器。混合器从一个或多个信源接收RTP 报文，对到达的数据报文进行重新同步和重新组合，这些重组的数据流被混合成一个数据流，将数据编码转化为在低带宽上可用的类型，并通过低速链路向低带宽区域转发。为了对多个输入信源进行统一的同步，混合器在多个媒体流之间进行定时调整，产生它自己的定时同步，因此所有从混合器输出的报文都把混合器作为同步信源。为了保证接收者能够正确识别混合器处理前的原始报文发送者，混合器在RTP报头中设置了CSRC标识符队列，以标识那些产生混和报文的原始同步信源。
// 转换器(Translator)
// 在Internet环境中，一些会议的参加者可能被隔离在应用级防火墙的外面，这些参加者被禁止直接使用 IP组播地址进行访问，虽然他们可能是通过高速链路连接的。在这些情况下，RTP允许使用转换器作为RTP级中继器。在防火墙两端分别安装一个转换器，防火墙之外的转换器过滤所有接收到的组播报文，并通过一条安全的连接传送给防火墙之内的转换器，内部转换器将这些组播报文再转发送给内部网络中的组播组成员。

func main() {
	log.Println("hi")
}