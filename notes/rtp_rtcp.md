RTP协议全解析（H264码流和PS流）
https://blog.csdn.net/chen495810242/article/details/39207305

RTP协议全解析（H264码流和PS流）
https://blog.csdn.net/chen495810242/article/details/39207305
RTP封装h264
https://blog.csdn.net/jwybobo2007/article/details/7054140
流媒体协议RTP、RTCP、H264详解
https://blog.csdn.net/fishmai/article/details/53676194
rtp对h264 nalu fu-a的处理
https://blog.csdn.net/occupy8/article/details/47067323
rtp h264注意点(FU-A分包方式说明)
https://blog.csdn.net/jwybobo2007/article/details/7235942
[live555] RTP包 NALU FU-A等之间的关系
https://blog.csdn.net/engineer_james/article/details/81745712

rfc1889     早期RTP协议，RTP v1
rfc1890     RTP的负载类型定义，对应于RTP v1
rfc2198     发送音频冗余数据的机制，FEC的雏形
rfc3550     现用RTP协议，RTP v2
rfc3551     RTP的负载类型定义，对应于RTP v2
rfc3611     RTCP的拓展报文即XR报文定义
rfc3640     RTP负载为MPEG-4的格式定义
rfc3711     RTP媒体流采用AES-128对称加密
rfc3984     RTP负载为H264的格式定义，已被6184取代
rfc4103     RTP负载为文本或者T.140的格式定义
rfc4585     NACK定义，通过实时的RTCP进行丢包重传
rfc4587     H261的负载定义
rfc4588     RTP重传包的定义
rfc4961     终端收发端口用同一个，叫做对称的RTP，便于DTLS加密
rfc5104     基于4585实时RTCP消息，来控制音视频编码器的机制
rfc5109     Fec的通用规范
rfc5124     SRTP的丢包重传
rfc5285     RTP 扩展头定义，可以扩展1或2个字节，比如CSRC，已被8285协议替代
rfc5450     计算RTP的时间差，可以配合抖动计算
rfc5484     RTP和RTCP中时间格式的定义
rfc5506     RTCP压缩
rfc5669     SRTP的对称加密算法的种子使用方法
rfc5691     对于MPEG-4中有多路音频的RTP负载格式的定义
rfc5760     RTCP对于单一源进行多播的反馈机制
rfc5761     RTP和RTCP在同一端口上传输
rfc6051     多RTP流的快速同步机制，适用于MCU的处理
rfc6128     RTCP对于多播中特定源的反馈机制
rfc6184     H264的负载定义
rfc6188     SRTP拓展定义AES192和AES256
rfc6189     ZRTP的定义，非对称加密，用于密钥交换
rfc6190     H264-SVC的负载定义
rfc6222     RTCP的CNAME的选定规则，可根据RFC 4122的UUID来选取
rfc6798     6843 6958 7002 7003 7097 RTCP的XR报文，关于各个方面的定义
rfc6904     SRTP的RTP头信息加密
rfc7022     RTCP的CNAME的选定规则，修订6222
rfc7160     RTP中的码流采样率变化的处理规则，音频较常见
rfc7164     RTP时间戳的校准机制
rfc7201     RTP的安全机制的建议，什么时候用DTLS，SRTP，ZRTP或者RTP over TLS等
rfc7202     RTP的安全机制的补充说明
rfc7656     RTP在webrtc中的应用场景
rfc7667     在MCU等复杂系统中，RTP流的设计规范
rfc7741     负载为vp8的定义
rfc7798     负载为HEVC的定义
rfc8082     基于4585实时RTCP消息，来控制分层的音视频编码器的机制，对于5104协议的补充
rfc8083     RTP的拥塞处理之码流环回的处理
rfc8108     单一会话，单一端口传输所有的RTP/RTCP码流，对现有RTP/RTCP机制的总结
rfc8285     RTP 扩展头定义，可以同时扩展为1或2个字节
