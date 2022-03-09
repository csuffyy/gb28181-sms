package main

import "log"

//系统时钟参考(SCR)是一个分两部分编码的42位字段。
//第一部分system_clock_reference_base是一个长度为33位
//第二部分system_clock_reference_extenstion是一个长度为9位
//SCR字段指出了基本流中包含ESCR_base最后一位的字节到达节目目标解码器输入端的期望时间。
// 4 + 1 + 5(40b) + 3(24b) + 2 = 15byte
type PsHeader struct {
	PackStartCode      uint32 // 32bit, 固定值 0x000001BA 表示包的开始
	Reversed0          uint8  // 2bit, 0x01
	ScrBase32_30       uint8  // 3bit, SystemClockReferenceBase32_30
	MarkerBit0         uint8  // 1bit, 标记位 0x1
	ScrBase29_15       uint16 // 15bit, 系统时钟参考基准
	MarkerBit1         uint8  // 1bit 标记位 0x1
	ScrBase14_0        uint16 // 15bit
	MarkerBit2         uint8  // 1bit 标记位 0x1
	ScrExtension       uint16 // 9bit, 系统时钟参考扩展, SystemClockReferenceExtension
	MarkerBit3         uint8  // 1bit 标记位 0x1
	ProgramMuxRate     uint32 // 22bit, 节目复合速率
	MarkerBit4         uint8  // 1bit 标记位 0x1
	MarkerBit5         uint8  // 1bit 标记位 0x1
	Reserved1          uint8  // 5bit
	PackStuffingLength uint8  // 3bit, 该字段后填充字节的个数
	StuffingByte       uint8  // 8bit, 填充字节 0xff
	SystemHeader
}

// 4 + 2 + 3 + 1 + 1 + 2 + 2 = 15byte
type PsSystemHeader struct {
	SystemHeaderStartCode     uint32 // 32bit, 固定值 0x000001BB
	HeaderLength              uint16 // 16bit, 表示后面还有多少字节
	MarkerBit0                uint8  // 1bit
	RateBound                 uint32 // 22bit, 速率界限, 取值不小于编码在节目流的任何包中的program_mux_rate字段的最大值。该字段可被解码器用于估计是否有能力对整个流解码。
	MarkerBit1                uint8  // 1bit
	AudioBound                uint8  // 6bit, 音频界限, 取值是在从0到32的闭区间中的整数
	FixedFlag                 uint8  // 1bit, 固定标志, 1表示比特率恒定, 0表示比特率可变
	CspsFlag                  uint8  // 1bit, 1表示节目流符合2.7.9中定义的限制
	SystemAudioLockFlag       uint8  // 1bit, 系统音频锁定标志, 1表示在系统目标解码器的音频采样率和system_clock_frequency之间存在规定的比率
	SystemVideoLockFlag       uint8  // 1bit, 系统视频锁定标志, 1表示在系统目标解码器的视频帧速率和system_clock_frequency之间存在规定的比率
	MarkerBit2                uint8  // 1bit
	VideoBound                uint8  // 5bit, 视频界限, 取值是在从0到16的闭区间中的整数
	PacketRateRestrictionFlag uint8  // 1bit, 分组速率限制, 若CSPS标识为'1'，则该字段表示2.7.9中规定的哪个限制适用于分组速率。若CSPS标识为'0'，则该字段的含义未定义
	ReservedBits              uint8  // 7bit, 保留位字段 0x7f
	StreamId                  uint8  // 8bit, 流标识, 指示其后的P-STD_buffer_bound_scale和P-STD_buffer_size_bound字段所涉及的流的编码和基本流号码。
	//若取值'1011 1000'，则其后的P-STD_buffer_bound_scale和P-STD_buffer_size_bound字段指节目流中所有的音频流。
	//若取值'1011 1001'，则其后的P-STD_buffer_bound_scale和P-STD_buffer_size_bound字段指节目流中所有的视频流。
	//若stream_id取其它值，则应该是大于或等于'1011 1100'的一字节值且应根据表2-18解释为流的编码和基本流号码。
	Reversed             uint8  // 2bit, 0x11
	PStdBufferBoundScale uint8  // 1bit, 缓冲区界限比例, 表示用于解释后续P-STD_buffer_size_bound字段的比例系数。若前面的stream_id表示一个音频流，则该字段值为'0'。若表示一个视频流，则该字段值为'1'。对于所有其它的流类型，该字段值可以为'0'也可以为'1'。
	PStdBufferSizeBound  uint16 // 13bit, 缓冲区大小界限, 若P-STD_buffer_bound_scale的值为'0'，则该字段以128字节为单位来度量缓冲区大小的边界。若P-STD_buffer_bound_scale的值为'1'，则该字段以1024字节为单位来度量缓冲区大小的边界。
}

//StreamType  uint8  // 8bit, 表示PES分组中的基本流且取值不能为0x05
//0x10	MPEG-4 视频流
//0x1B	H.264 视频流
//0x24  H.265 视频流, ISO/IEC 13818-1:2018 增加了这个
//0x80	SVAC 视频流
//0x90	G.711 音频流
//0x92	G.722.1 音频流
//0x93	G.723.1 音频流
//0x99	G.729 音频流
//0x9B	SVAC音频流
//ElementaryStreamId uint8  // 8bit 指出PES分组中stream_id字段的值
//0x(C0~DF)指音频
//0x(E0~EF)为视频
// StreamType && ElementaryStreamId 可以判断是 h264 还是 h265
// StreamType == 0x1b && ElementaryStreamId == 0xe0 这个是h264
// StreamType == 0x24 && ElementaryStreamId == 0xe0 这个是h265
// 1 + 1 + 2 = 4byte
type StreamMap struct {
	StreamType                 uint8  // 8bit, 表示PES分组中的基本流且取值不能为0x05
	ElementaryStreamId         uint8  // 8bit, 指出PES分组中stream_id字段的值, 其中0x(C0~DF)指音频, 0x(E0~EF)为视频
	ElementaryStreamInfoLength uint16 // 16bit, 指出紧跟在该字段后的描述的字节长度
}

// 节目流映射
// 4 + 2 + 2 + 4 + 4*n + 4 = 20byte
type PsMap struct {
	PacketStartCodePrefix     uint32      // 24bit, 固定值 0x000001
	MapStreamId               uint8       // 8bit, 映射流标识 值为0xBC
	ProgramStreamMapLength    uint16      // 16bit, 表示后面还有多少字节, 该字段的最大值为0x3FA(1018)
	CurrentNextIndicator      uint8       // 1bit, 1表示当前可用, 0表示下个可用
	Reserved0                 uint8       // 2bit
	ProgramStreamMapVersion   uint8       // 5bit, 表示整个节目流映射的版本号, 节目流映射的定义发生变化，该字段将递增1，并对32取模
	Reserved1                 uint8       // 7bit
	MarkerBit                 uint8       // 1bit
	ProgramStreamInfoLength   uint16      // 16bit, 紧跟在该字段后的描述符的总长度
	ElementaryStreamMapLength uint16      // 16bit, 基本流映射长度, StreamMap的长度
	StreamMap                 []StreamMap // 32bit, 基本流信息
	CRC32                     uint32      // 32bit
}

// 针对H264 做如下PS 封装：每个IDR NALU 前一般都会包含SPS、PPS 等NALU，因此将SPS、PPS、IDR 的NALU 封装为一个PS 包，包括ps 头，然后加上PS system header，PS system map，PES header+h264 raw data。
// 所以一个IDR NALU PS 包由外到内顺序是：PSheader| PS system header | PS system Map | PES header | h264 raw data。
// 对于其它非关键帧的PS 包，就简单多了，直接加上PS头和PES 头就可以了。顺序为：PS header | PES header | h264raw data。
// 以上是对只有视频video 的情况，如果要把音频Audio也打包进PS 封装，也可以。
// 当有音频数据时，将数据加上PES header 放到视频PES 后就可以了。
// 顺序如下：PS 包=PS头|PES(video)|PES(audio)，再用RTP 封装发送就可以了。
func main() {
	log.Println("hi")
}
