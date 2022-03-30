package main

import (
	"container/list"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"strings"
	"utils"
)

const (
	H264ClockFrequency = 90 // ISO/IEC13818-1中指定, 时钟频率为90kHz
	TsPacketLen        = 188
	PatPid             = 0x0
	PmtPid             = 0x1001
	VideoPid           = 0x100
	AudioPid           = 0x101
	VideoStreamId      = 0xe0
	AudioStreamId      = 0xc0
)

type HlsInfo struct {
	LogHlsFn     string      // 文件名 包括路径
	logHls       *log.Logger // 每个发布者、播放者的日志都是独立的
	M3u8Path     string      // m3u8文件路径, 包含文件名
	M3u8File     *os.File    // m3u8文件描述符
	M3u8Data     string      // m3u8内容
	TsNum        uint32      // m3u8里ts的个数
	TsFirstSeq   uint32      // m3u8里第一个ts的序号
	TsLastSeq    uint32      // m3u8里最后一个ts的序号
	TsList       *list.List  // 存储ts内容, 双向链表, 删头追尾
	TsFirstTs    uint32      // ts文件中第一个时间戳
	TsExtInfo    float64     // ts文件的播放时长
	TsPath       string      // ts文件路径, 包含文件名
	TsFile       *os.File    // ts文件描述符
	TsData       []byte      // ts文件内容(不完整，正在生成)
	VideoCounter uint8       // 4bit, 0x0 - 0xf 循环
	AudioCounter uint8       // 4bit, 0x0 - 0xf 循环
	SpsPpsData   []byte      // 视频关键帧tsPacket
	AdtsData     []byte      // 音频tsPacket需要
}

/**********************************************************/
/* tsFile里 tsPacket的顺序和结构
/**********************************************************/
// rtmp流如何生成ts? 详见 notes/tsFormat.md (必看)
// 第1个tsPacket内容为: tsHeader + 0x00 + pat
// 第2个tsPacket内容为: tsHeader + 0x00 + pmt
// *** 每个关键帧都要有sps和pps
// *** 关键帧的 PesPacketLength == 0x0000
// 第3个tsPacket内容为: tsHeader + adaptation(pcr) + pesHeader + 0x00000001 + 0x09 + 0xf0 + 0x00000001 + 0x67 + sps + 0x00000001 + 0x68 + pps + 0x00000001 + 0x65 + keyFrame
// 第4个tsPacket内容为: tsHeader + keyFrame
// ...
// 第388个tsPacket内容为: tsHeader + pesHeader + 0x00000001 + 0x09 + 0xf0 + 0x00000001 + 0x61 + interFrame
// 第389个tsPacket内容为: tsHeader + interFrame
// ...
// 第481个tsPacket内容为: tsHeader + pesHeader + adts + aacFrame
// 第482个tsPacket内容为: tsHeader + aacFrame
// ...

// 0x00000001 或 0x000001 是NALU单元的开始码
//NalRefIdc        uint8 // 2bit, 简写为NRI
//似乎指示NALU的重要性, 如00的NALU解码器可以丢弃它而不影响图像的回放,取值越大, 表示当前NAL越重要, 需要优先受到保护.
//NalUnitType      uint8 // 5bit, 简写为Type
// nal_unit_type	1-23	表示单一Nal单元模式
// nal_unit_type	24-27	表示聚合Nal单元模式, 本类型用于聚合多个NAL单元到单个RTP荷载中
// nal_unit_type	28-29	表示分片Nal单元模式, 将NALU 单元拆分到多个RTP包中发送
// 0, Reserved
// 1, 非关键帧
// 2, 非IDR图像中A类数据划分片段
// 3, 非IDR图像中B类数据划分片段
// 4, 非IDR图像中C类数据划分片段
// 5, 关键帧
// 6, SEI 补充增强信息
// 7, SPS 序列参数集
// 8, PPS 图像参数集
// 9, 分隔符, 后跟1字节 0xf0
// 10, 序列结束
// 11, 码流结束
// 12, 填充
// 13...23, 保留
// 24, STAP-A 单时间聚合包类型A
// 25, STAP-B 单时间聚合包类型B
// 26, MTAP16 多时间聚合包类型(MTAP)16位位移
// 27, MTAP24 多时间聚合包类型(MTAP)24位位移
// 28, FU-A 单个NALU size 大于 MTU 时候就要拆分 使用FU-A
// 29, FU-B 不常用
// 30-31 Reserved
// +---------------+
// |0|1|2|3|4|5|6|7|
// +-+-+-+-+-+-+-+-+
// |F|NRI|  Type   |
// +---------------+
// 1 + 2 + 5 = 1byte
type NaluHeader struct {
	ForbiddenZeroBit uint8 // 1bit, 简写为F
	NalRefIdc        uint8 // 2bit, 简写为NRI, NalUnitType = 5 或者 7 8 的时候 NRI必须是11
	NalUnitType      uint8 // 5bit, 简写为Type
}

/**********************************************************/
/* prepare SpsPpsData and AdtsData
/**********************************************************/
//0x00000001 + 0x67 + sps + 0x00000001 + 0x68 + pps
func PrepareSpsPpsData(s *Stream, c *Chunk) {
	var AvcC AVCDecoderConfigurationRecord
	AvcC.ConfigurationVersion = c.MsgData[5]          // 8bit, 0x01
	AvcC.AVCProfileIndication = c.MsgData[6]          // 8bit, 0x4d, 0100 1101
	AvcC.ProfileCompatibility = c.MsgData[7]          // 8bit, 0x40, 0100 0000
	AvcC.AVCLevelIndication = c.MsgData[8]            // 8bit, 0x1f
	AvcC.Reserved0 = (c.MsgData[9] & 0xFC) >> 2       // 6bit, 0xff, 1111 1111
	AvcC.LengthSizeMinuxOne = c.MsgData[9] & 0x3      // 2bit, 0xff
	AvcC.Reserved1 = (c.MsgData[10] & 0xE0) >> 5      // 3bit, 0xe1, 11100001
	AvcC.NumOfSps = c.MsgData[10] & 0x1F              // 5bit, 0xe1
	AvcC.SpsSize = ByteToUint16(c.MsgData[11:13], BE) // 16bit, 0x001c
	EndPos := 13 + uint16(AvcC.NumOfSps)*AvcC.SpsSize // 13 + 1 * 28
	AvcC.SpsData = c.MsgData[13:EndPos]               // 28Byte
	AvcC.NumOsPps = c.MsgData[EndPos]                 // 8bit, 0x01
	AvcC.PpsSize =
		ByteToUint16(c.MsgData[EndPos+1:EndPos+3], BE) // 16bit, 0x0004
	AvcC.PpsData = c.MsgData[EndPos+3:] // 4Byte
	s.logHls.Printf("%#v", AvcC)

	//sps内容里第一个就是0x67, pps内容里第一个就是0x68
	/*
		size := 8 + AvcC.SpsSize + AvcC.PpsSize
		s.SpsPpsData = make([]byte, size)
		Uint32ToByte(0x00000001, s.SpsPpsData[0:4], BE)
		copy(s.SpsPpsData[4:4+AvcC.SpsSize], AvcC.SpsData)
		sp := 4 + AvcC.SpsSize
		Uint32ToByte(0x00000001, s.SpsPpsData[sp:sp+4], BE)
		copy(s.SpsPpsData[sp+4:sp+4+AvcC.PpsSize], AvcC.PpsData)
		s.logHls.Printf("SpsPpsData: %x", s.SpsPpsData)
	*/

	//sps内容里第一个就是0x67, pps内容里第一个就是0x68
	size := 6 + AvcC.SpsSize + AvcC.PpsSize
	s.SpsPpsData = make([]byte, size)
	Uint24ToByte(0x000001, s.SpsPpsData[0:3], BE)
	copy(s.SpsPpsData[3:3+AvcC.SpsSize], AvcC.SpsData)
	sp := 3 + AvcC.SpsSize
	Uint24ToByte(0x000001, s.SpsPpsData[sp:sp+3], BE)
	copy(s.SpsPpsData[sp+3:sp+3+AvcC.PpsSize], AvcC.PpsData)
	s.logHls.Printf("SpsPpsData: %x", s.SpsPpsData)
}

// FF F9 50 80 2E 7F FC
// 11111111 11111001 01010000 10000000 00101110 01111111 11111100
// fff 1 00 1 01 0100 0 010 0 0 0 0 0000101110011 11111111111 00
// 366-2=364, 371-264=7, 7字节adts  0x173 = 371
//ProfileObjectType            uint8  // 2bit
// 0	Main profile
// 1	Low Complexity profile(LC)
// 2	Scalable Sampling Rate profile(SSR)
// 3	(reserved)
//SamplingFrequencyIndex       uint8  // 4bit, 使用的采样率下标
// 0: 96000 Hz
// 1: 88200 Hz
// 2: 64000 Hz
// 3: 48000 Hz
// 4: 44100 Hz
// 5: 32000 Hz
// 6: 24000 Hz
// 7: 22050 Hz
// 8: 16000 Hz
// 9: 12000 Hz
// 10: 11025 Hz
// 11: 8000 Hz
// 12: 7350 Hz
// 13: Reserved
// 14: Reserved
// 15: frequency is written explictly
// ADTS 定义在 ISO 14496-3, P122
// 固定头信息 + 可变头信息(home之后，不包括home)
//28bit固定头 + 28bit可变头 = 56bit, 7byte
type Adts struct {
	Syncword                     uint16 // 12bit, 固定值0xfff
	Id                           uint8  // 1bit, 固定值0x1, MPEG Version: 0 is MPEG-4, 1 is MPEG-2
	Layer                        uint8  // 2bit, 固定值00
	ProtectionAbsent             uint8  // 1bit, 0表示有CRC校验, 1表示没有CRC校验
	ProfileObjectType            uint8  // 2bit, 表示使用哪个级别的AAC，有些芯片只支持AAC LC
	SamplingFrequencyIndex       uint8  // 4bit, 使用的采样率下标
	PrivateBit                   uint8  // 1bit, 0x0
	ChannelConfiguration         uint8  // 3bit, 表示声道数
	OriginalCopy                 uint8  // 1bit
	Home                         uint8  // 1bit
	CopyrightIdentificationBit   uint8  // 1bit
	CopyrightIdentificationStart uint8  // 1bit
	AacFrameLength               uint16 // 13bit, adts头长度 + aac数据长度
	AdtsBufferFullness           uint16 // 11bit, 固定值0x7ff, 表示码率可变
	NumberOfRawDataBlocksInFrame uint8  // 2bit
}

// ffmpeg-4.4.1/libavcodec/adts_header.c
// ff_adts_header_parse() ffmpeg中解析adts的代码
func PrepareAdtsData(s *Stream, c *Chunk) {
	var AacC AudioSpecificConfig
	AacC.ObjectType = (c.MsgData[2] & 0xF8) >> 3 // 5bit
	AacC.SamplingIdx =
		((c.MsgData[2] & 0x7) << 1) | (c.MsgData[3] >> 7) // 4bit
	AacC.ChannelNum = (c.MsgData[3] & 0x78) >> 3     // 4bit
	AacC.FrameLenFlag = (c.MsgData[3] & 0x4) >> 2    // 1bit
	AacC.DependCoreCoder = (c.MsgData[3] & 0x2) >> 1 // 1bit
	AacC.ExtensionFlag = c.MsgData[3] & 0x1          // 1bit
	// 2, 4, 2, 0(1024), 0, 0
	s.logHls.Printf("%#v", AacC)

	//ff f9 50 80 00 ff fc  //自己测试文件自己代码生成的
	// 11111111 11111001 01010000 10000000 00000000 11111111 11111100
	// fff 1 00 1 01 0100 0 010 0 0 0 0 0000000000111 11111111111 00
	//FF F9 68 40 5C FF FC  //别人测试ts文件直接读取的
	// 11111111 11111001 01101000 01000000 01011100 11111111 11111100
	// fff 1 00 1 01 1010 0 001 0 0 0 0 0001011100111 11111111111 00
	var adts Adts
	adts.Syncword = 0xfff
	adts.Id = 0x1 // 1bit, MPEG Version: 0 is MPEG-4, 1 is MPEG-2
	adts.Layer = 0x0
	adts.ProtectionAbsent = 0x1
	adts.ProfileObjectType = AacC.ObjectType - 1
	adts.SamplingFrequencyIndex = AacC.SamplingIdx
	adts.PrivateBit = 0x0
	adts.ChannelConfiguration = AacC.ChannelNum
	adts.OriginalCopy = 0x0
	adts.Home = 0x0
	adts.CopyrightIdentificationBit = 0x0
	adts.CopyrightIdentificationStart = 0x0
	// 这里不知道aac数据长度, 所以先复制为0x7
	adts.AacFrameLength = 0x7
	adts.AdtsBufferFullness = 0x7ff
	adts.NumberOfRawDataBlocksInFrame = 0x0
	s.logHls.Printf("%#v", adts)

	s.AdtsData = make([]byte, 7)
	s.AdtsData[0] = 0xff
	s.AdtsData[1] = 0xf0 | (adts.Id&0x1)<<3 | (adts.Layer&0x3)<<1 | (adts.ProtectionAbsent & 0x1)
	s.AdtsData[2] = (adts.ProfileObjectType&0x3)<<6 | (adts.SamplingFrequencyIndex&0xf)<<2 | (adts.PrivateBit&0x1)<<1 | (adts.ChannelConfiguration&0x4)>>2
	s.AdtsData[3] = (adts.ChannelConfiguration&0x3)<<6 | (adts.OriginalCopy&0x1)<<5 | (adts.Home&0x1)<<4 | (adts.CopyrightIdentificationBit&0x1)<<3 | (adts.CopyrightIdentificationStart&0x1)<<2 | uint8((adts.AacFrameLength>>11)&0x3)
	s.AdtsData[4] = uint8((adts.AacFrameLength >> 3) & 0xff)
	s.AdtsData[5] = uint8(adts.AacFrameLength&0x7)<<5 | uint8((adts.AdtsBufferFullness>>6)&0x1f)
	s.AdtsData[6] = uint8((adts.AdtsBufferFullness&0x3f)<<2) | (adts.NumberOfRawDataBlocksInFrame & 0x3)
	s.logHls.Printf("AdtsData: %x", s.AdtsData)
}

func ParseAdtsData(s *Stream) Adts {
	var adts Adts
	data := s.AdtsData
	adts.Syncword = uint16(data[0])<<4 | uint16(data[1])>>4
	adts.Id = (data[1] >> 3) & 0x1
	adts.Layer = (data[1] >> 1) & 0x3
	adts.ProtectionAbsent = data[1] & 0x1
	adts.ProfileObjectType = (data[2] >> 6) & 0x3
	adts.SamplingFrequencyIndex = (data[2] >> 2) & 0xf
	adts.PrivateBit = (data[2] >> 1) & 0x1
	adts.ChannelConfiguration = (data[2]&0x1)<<2 | (data[3]>>6)&0x3
	adts.OriginalCopy = (data[3] >> 5) & 0x1
	adts.Home = (data[3] >> 4) & 0x1
	adts.CopyrightIdentificationBit = (data[3] >> 3) & 0x1
	adts.CopyrightIdentificationStart = (data[3] >> 2) & 0x1
	adts.AacFrameLength = (uint16(data[3])&0x3)<<11 | uint16(data[4])<<3 | (uint16(data[5])>>5)&0x7
	adts.AdtsBufferFullness = (uint16(data[5])&0x1f)<<6 | (uint16(data[6])>>2)&0x3f
	adts.NumberOfRawDataBlocksInFrame = data[6] & 0x3
	s.logHls.Printf("%#v", adts)
	return adts
}

// size = adts头(7字节) + aac数据长度
// 函数A 把[]byte 传给函数B, B修改后 A里的值也会变
func SetAdtsLength(d []byte, size uint16) {
	d[3] = (d[3] & 0xfc) | uint8((size>>11)&0x2) // 最右2bit
	d[4] = (d[4] & 0x00) | uint8((size>>3)&0xff) // 8bit
	d[5] = (d[5] & 0x1f) | uint8((size&0x7)<<5)  // 最左3bit
}

/**********************************************************/
/* HlsCreator()
/**********************************************************/
func HlsCreator(s *Stream) {
	// 初始化hls的生产
	s.LogHlsFn = fmt.Sprintf("%s%s/%s_hlsCreator_%s.log", conf.LogStreamPath, s.Key, s.Key, s.RemoteAddr)
	s.logHls, _ = StreamLogCreate(s.LogHlsFn)

	folder := fmt.Sprintf("%s%s", conf.HlsSavePath, s.Key)
	err := os.MkdirAll(folder, 0755)
	if err != nil {
		log.Println(err)
		return
	}
	s.M3u8Path = fmt.Sprintf("%s/%s.m3u8", folder, s.Key)
	s.logHls.Println("m3u8Path is", s.M3u8Path)
	s.M3u8File, err = os.OpenFile(s.M3u8Path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		log.Println(err)
		return
	}

	s.HlsInfo.TsList = list.New()

	var i uint32 = 0
	for {
		c, ok := <-s.HlsChan
		if !ok {
			s.logHls.Printf("%s HlsCreator stop", s.Key)
			return
		}
		s.logHls.Printf("-------------------->> chunk %d", i)
		i++
		s.logHls.Printf("===>> fmt=%d, csid=%d, timestamp=%d, MsgLength=%d, MsgTypeId=%d, DataType=%s", c.Fmt, c.Csid, c.Timestamp, c.MsgLength, c.MsgTypeId, c.DataType)

		switch c.DataType {
		case "Metadata":
			continue
		case "AudioAacFrame":
			//continue
		case "VideoHeader":
			PrepareSpsPpsData(s, c)
			continue
		case "AudioHeader":
			PrepareAdtsData(s, c)
			ParseAdtsData(s)
			continue
		}

		tf := TsCreate(s, c)
		if tf {
			//M3u8Update(s, c)
		}
	}
}

/**********************************************************/
/* ts
/**********************************************************/
//===> PID
//0x0000	表示PAT
//0x0001	表示CAT
//0x1fff	表示空包
//===> AdaptationFieldControl
//0x0	是保留值
//0x1	无调整字段，仅含有效负载
//0x2	仅含调整字段，无有效负载
//0x3	调整字段后含有效负载
// 8 + 3 + 13 + 2 + 2 + 4 = 4byte
type TsHeader struct {
	SyncByte                   uint8  // 8bit, 同步字节 固定值0x47, 后面的数据是不会出现0x47的
	TransportErrorIndicator    uint8  // 1bit, 传输错误标志, 一般传输错误的话就不会处理这个包了
	PayloadUnitStartIndicator  uint8  // 1bit, 负载单元开始标志
	TransportPriority          uint8  // 1bit, 传输优先级, 1表示高优先级
	PID                        uint16 // 13bit, TS包的数据类型
	TransportScramblingControl uint8  // 2bit, 传输加扰控制, 00表示未加密
	AdaptationFieldControl     uint8  // 2bit, 适应域控制
	ContinuityCounter          uint8  // 4bit, 连续计数器, 0x0-0xf循环
}

// 8 + 8  = 16bit, 2Byte
type Adaptation struct {
	AdaptationFieldLength             uint8 // 8bit
	DiscontinuityIndicator            uint8 // 1bit
	RandomAccessIndicator             uint8 // 1bit
	ElementaryStreamPriorityIndicator uint8 // 1bit
	PcrFlag                           uint8 // 1bit
	OpcrFlag                          uint8 // 1bit
	SplicingPointFlag                 uint8 // 1bit
	TransportPrivateDataFlag          uint8 // 1bit
	AdaptationFieldExtensionFlag      uint8 // 1bit
}

//标准规定在原始音频和视频流中，PTS的间隔不能超过0.7s，而出现在TS包头的PCR间隔不能超过0.1s。
//假设a，b两人约定某个时刻去做某事，则需要一个前提，他们两人的手表必须是同步的，比如都是使用北京时间，如果他们的表不同步，就会错过约定时刻。pcr就是北京时间，编码器将自己的系统时钟采样，以pcr形式放入ts，解码器使用pcr同步自己的系统时钟，保证编码器和解码器的时钟同步。
// PCR 系统参考时钟, PCR 是 TS 流中才有的概念, 用于恢复出与编码端一致的系统时序时钟STC（System Time Clock）
// PCR多长时间循环一次, (0x1FFFFFFFF*300+299)/27000000/3600 约为 26.5 小时
// 33 + 6 + 9 = 48bit, 6Byte
type Pcr struct {
	ProgramClockReferenceBase      uint64 // 33bit
	Reserved                       uint8  // 6bit
	ProgramClockReferenceExtension uint16 // 9bit
}

// PCR的插入必须在PCR字段的最后离开复用器的那一时刻，同时把27MHz系统时钟的采样瞬时值作为PCR字段插入到相应的PCR域。是放在TS包头的自适应区中传送。27 MHz时钟经波形整理后分两路，一路是由27MHz脉冲直接触发计数器生成扩展域PCR_ext，长度为9bits。另一路经一个300分频器后的90 kHz脉冲送入一个33位计数器生成90KHZ基值，列入PCR_base（基值域），长度33bits，用于和PTS/DTS比较，产生解码和显示所需要的同步信号。这两部分被置入PCR域，共同组成了42位的PCR。

//---------------------------------------------------------/
// tsPakcet
//---------------------------------------------------------/
// tsPacket 大小固定188byte, tsHeader 固定4byte
// TsPacketCreate() 只有tsHeader + 184字节数据
// 返回tsData 和 写入tsData的字节数(也就是data数据消耗了多少)
func TsPacketCreate(s *Stream, pid uint16, data []byte) ([]byte, int) {
	dataLen := len(data)
	freeBuffLen := 188 - 4

	var th TsHeader
	th.SyncByte = 0x47
	th.TransportErrorIndicator = 0x0
	th.PayloadUnitStartIndicator = 0x0
	th.TransportPriority = 0x0
	th.PID = pid
	th.TransportScramblingControl = 0x0
	th.AdaptationFieldControl = 0x1
	if dataLen < freeBuffLen {
		th.AdaptationFieldControl = 0x3
	}
	if pid == AudioPid {
		th.ContinuityCounter = s.AudioCounter
		s.AudioCounter++
		if s.AudioCounter > 0xf {
			s.AudioCounter = 0x0
		}
		//s.logHls.Printf("th.ContinuityCounter=%x", th.ContinuityCounter)
	}
	if pid == VideoPid {
		th.ContinuityCounter = s.VideoCounter
		s.VideoCounter++
		if s.VideoCounter > 0xf {
			s.VideoCounter = 0x0
		}
	}

	tsData := make([]byte, 188)
	tsData[0] = th.SyncByte
	tsData[1] = (th.TransportErrorIndicator&0x1)<<7 | (th.PayloadUnitStartIndicator&0x1)<<6 | (th.TransportPriority&0x1)<<5 | uint8((th.PID&0x1f00)>>8)
	tsData[2] = uint8(th.PID & 0xff)
	tsData[3] = (th.TransportScramblingControl&0x3)<<6 | (th.AdaptationFieldControl&0x3)<<4 | (th.ContinuityCounter & 0xf)

	//s.logHls.Printf("dataLen=%d, freeBuffLen=%d", dataLen, freeBuffLen)
	if dataLen >= freeBuffLen {
		dataLen = freeBuffLen
		copy(tsData[4:4+dataLen], data)
	} else {
		// 添加 adaptation(2字节) 填充 0xff
		// 183 184 -> 185 184 这种情况 无法添加adaptation 需特殊处理
		// 182 184 -> 184 184 这种情况 刚好添加adaptation
		// 181 184 -> 183 184 这种情况 添加adaptation 还能填充1个0xff
		// 180 184 -> 182 184 这种情况 添加adaptation 还能填充2个0xff
		// 188 = 4 + 2 + padLen + dataLen
		padLen := 188 - 4 - 2 - dataLen
		tsData[4] = uint8(padLen + 1)
		if padLen < 0 {
			padLen = 0
			dataLen -= 1
			tsData[4] = 0x1
		}
		tsData[5] = 0x0
		//s.logHls.Printf("padLen=%d, tsData[4]=%d, tsData[5]=%d", padLen, tsData[4], tsData[5])
		for i := 0; i < padLen; i++ {
			tsData[6+i] = 0xff
		}
		copy(tsData[6+padLen:], data)
	}
	return tsData, dataLen
}

// TsPacketCreatePatPmt() tsHeader和pat/pmt之间用0x00分割
func TsPacketCreatePatPmt(s *Stream, pid uint16, data []byte) ([]byte, int) {
	var th TsHeader
	th.SyncByte = 0x47                  // 8bit
	th.TransportErrorIndicator = 0x0    // 1bit
	th.PayloadUnitStartIndicator = 0x1  // 1bit
	th.TransportPriority = 0x0          // 1bit
	th.PID = pid                        // 13bit
	th.TransportScramblingControl = 0x0 // 2bit
	th.AdaptationFieldControl = 0x1     // 2bit
	th.ContinuityCounter = 0x0          // 4bit

	tsData := make([]byte, 188)
	tsData[0] = th.SyncByte
	tsData[1] = (th.TransportErrorIndicator&0x1)<<7 | (th.PayloadUnitStartIndicator&0x1)<<6 | (th.TransportPriority&0x1)<<5 | uint8((th.PID&0x1f00)>>8)
	tsData[2] = uint8(th.PID & 0xff)
	tsData[3] = (th.TransportScramblingControl&0x3)<<6 | (th.AdaptationFieldControl&0x3)<<4 | (th.ContinuityCounter & 0xf)

	// ts和pat之间有一个字节的分隔符
	// ts和pmt之间有一个字节的分隔符
	// ts和pes之间没有一个字节的分隔符
	tsData[4] = 0x0

	dataLen := len(data)
	freeBuffLen := 188 - 5
	if dataLen >= freeBuffLen {
		dataLen = freeBuffLen
		copy(tsData[5:5+dataLen], data)
	} else {
		copy(tsData[5:5+dataLen], data)
		freeBuffLen = 188 - 5 - dataLen
		for i := 0; i < freeBuffLen; i++ {
			tsData[5+dataLen+i] = 0xff
		}
	}

	//s.logHls.Printf("%x", tsData)
	s.logHls.Printf("TsHeaderLen=4, SeparatorLen=1, DataLen=%d, PaddingLen=%d", dataLen, freeBuffLen)
	return tsData, dataLen
}

// tsHeader + adaptation(pcr) + pesHeader + sps + pps + keyFrmae
// TsPacketCreateKeyFrame() 视频关键帧的第一个tsPacket
func TsPacketCreateKeyFrame(s *Stream, pid uint16, data []byte, pcr uint64) ([]byte, int) {
	var th TsHeader
	th.SyncByte = 0x47
	th.TransportErrorIndicator = 0x0
	th.PayloadUnitStartIndicator = 0x1
	th.TransportPriority = 0x0
	th.PID = pid
	th.TransportScramblingControl = 0x0
	th.AdaptationFieldControl = 0x3
	th.ContinuityCounter = s.VideoCounter
	s.VideoCounter++
	if s.VideoCounter > 0xf {
		s.VideoCounter = 0x0
	}

	var a Adaptation
	a.AdaptationFieldLength = 0x7
	a.DiscontinuityIndicator = 0x0
	a.RandomAccessIndicator = 0x0
	a.ElementaryStreamPriorityIndicator = 0x0
	a.PcrFlag = 0x1
	a.OpcrFlag = 0x0
	a.SplicingPointFlag = 0x0
	a.TransportPrivateDataFlag = 0x0
	a.AdaptationFieldExtensionFlag = 0x0

	tsData := make([]byte, 188)
	tsData[0] = th.SyncByte
	tsData[1] = (th.TransportErrorIndicator&0x1)<<7 | (th.PayloadUnitStartIndicator&0x1)<<6 | (th.TransportPriority&0x1)<<5 | uint8((th.PID&0x1f00)>>8)
	tsData[2] = uint8(th.PID & 0xff)
	tsData[3] = (th.TransportScramblingControl&0x3)<<6 | (th.AdaptationFieldControl&0x3)<<4 | (th.ContinuityCounter & 0xf)

	tsData[4] = a.AdaptationFieldLength
	tsData[5] = (a.DiscontinuityIndicator&0x1)<<7 | (a.RandomAccessIndicator&0x1)<<6 | (a.ElementaryStreamPriorityIndicator&0x1)<<5 | (a.PcrFlag&0x1)<<4 | (a.OpcrFlag&0x1)<<3 | (a.SplicingPointFlag&0x1)<<2 | (a.TransportPrivateDataFlag&0x1)<<1 | (a.AdaptationFieldExtensionFlag & 0x1)

	tsData[6] = uint8((pcr >> 25) & 0xff)
	tsData[7] = uint8((pcr >> 17) & 0xff)
	tsData[8] = uint8((pcr >> 9) & 0xff)
	tsData[9] = uint8((pcr >> 1) & 0xff)
	tsData[10] = uint8(((pcr & 0x1) << 7) | 0x7e)
	tsData[11] = 0x00

	dataLen := len(data)
	freeBuffLen := 188 - 12
	if dataLen >= freeBuffLen {
		dataLen = freeBuffLen
		copy(tsData[12:12+dataLen], data)
	} else {
		copy(tsData[12:12+dataLen], data)
		freeBuffLen = 188 - 12 - dataLen
		for i := 0; i < freeBuffLen; i++ {
			tsData[12+dataLen+i] = 0xff
		}
	}
	return tsData, dataLen
}

// tsHeader + pesHeader + interFrame
// TsPacketCreateInterFrame() 视频非关键帧的第一个tsPacket
func TsPacketCreateInterFrame(s *Stream, pid uint16, data []byte) ([]byte, int) {
	var th TsHeader
	th.SyncByte = 0x47
	th.TransportErrorIndicator = 0x0
	th.PayloadUnitStartIndicator = 0x1
	th.TransportPriority = 0x0
	th.PID = pid
	th.TransportScramblingControl = 0x0
	th.AdaptationFieldControl = 0x1
	th.ContinuityCounter = s.VideoCounter
	s.VideoCounter++
	if s.VideoCounter > 0xf {
		s.VideoCounter = 0x0
	}

	tsData := make([]byte, 188)
	tsData[0] = th.SyncByte
	tsData[1] = (th.TransportErrorIndicator&0x1)<<7 | (th.PayloadUnitStartIndicator&0x1)<<6 | (th.TransportPriority&0x1)<<5 | uint8((th.PID&0x1f00)>>8)
	tsData[2] = uint8(th.PID & 0xff)
	tsData[3] = (th.TransportScramblingControl&0x3)<<6 | (th.AdaptationFieldControl&0x3)<<4 | (th.ContinuityCounter & 0xf)

	dataLen := len(data)
	freeBuffLen := 188 - 4
	if dataLen >= freeBuffLen {
		dataLen = freeBuffLen
		copy(tsData[4:4+dataLen], data)
	} else {
		copy(tsData[4:4+dataLen], data)
		freeBuffLen = 188 - 4 - dataLen
		for i := 0; i < freeBuffLen; i++ {
			tsData[i+4+dataLen] = 0xff
		}
	}
	return tsData, dataLen
}

// tsHeader + pesHeader + adts + aacFrame
// TsPacketCreateAacFrame() 音频帧的第一个tsPacket
func TsPacketCreateAacFrame(s *Stream, pid uint16, data []byte) ([]byte, int) {
	dataLen := len(data)
	freeBuffLen := 188 - 4

	var th TsHeader
	th.SyncByte = 0x47
	th.TransportErrorIndicator = 0x0
	th.PayloadUnitStartIndicator = 0x1
	th.TransportPriority = 0x0
	th.PID = pid
	th.TransportScramblingControl = 0x0
	th.AdaptationFieldControl = 0x1
	if dataLen < freeBuffLen {
		th.AdaptationFieldControl = 0x3
	}
	th.ContinuityCounter = s.AudioCounter
	s.AudioCounter++
	if s.AudioCounter > 0xf {
		s.AudioCounter = 0x0
	}

	tsData := make([]byte, 188)
	tsData[0] = th.SyncByte
	tsData[1] = (th.TransportErrorIndicator&0x1)<<7 | (th.PayloadUnitStartIndicator&0x1)<<6 | (th.TransportPriority&0x1)<<5 | uint8((th.PID&0x1f00)>>8)
	tsData[2] = uint8(th.PID & 0xff)
	tsData[3] = (th.TransportScramblingControl&0x3)<<6 | (th.AdaptationFieldControl&0x3)<<4 | (th.ContinuityCounter & 0xf)

	if dataLen >= freeBuffLen {
		dataLen = freeBuffLen
		copy(tsData[4:4+dataLen], data)
	} else {
		padLen := 188 - 4 - 2 - dataLen
		tsData[4] = uint8(padLen + 1)
		if padLen < 0 {
			padLen = 0
			dataLen -= 1
			tsData[4] = 0x1
		}
		tsData[5] = 0x0
		for i := 0; i < padLen; i++ {
			tsData[6+i] = 0xff
		}
		copy(tsData[6+padLen:], data)
	}
	return tsData, dataLen
}

//---------------------------------------------------------/
// tsFile
//---------------------------------------------------------/
// xxx.ts文件 有很多个 188字节的ts包 组成
func TsFileCreate(s *Stream, c *Chunk) {
	if s.TsPath != "" {
		s.TsFile.Close()
		M3u8Update(s, c)
	}

	s.TsPath = fmt.Sprintf("%s%s/%s_%d.ts", conf.HlsSavePath, s.Key, s.Key, s.TsLastSeq)
	s.logHls.Println(s.TsPath)

	var err error
	s.TsFile, err = os.OpenFile(s.TsPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		log.Println(err)
		s.TsPath = ""
		return
	}

	_, patData := PatCreate()
	s.TsData, _ = TsPacketCreatePatPmt(s, PatPid, patData)
	_, err = s.TsFile.Write(s.TsData)
	if err != nil {
		s.logHls.Printf("Write ts fail, %s", err)
		return
	}

	_, pmtData := PmtCreate()
	s.TsData, _ = TsPacketCreatePatPmt(s, PmtPid, pmtData)
	_, err = s.TsFile.Write(s.TsData)
	if err != nil {
		s.logHls.Printf("Write ts fail, %s", err)
		return
	}

	TsFileAppend(s, c)
	s.TsFirstTs = c.Timestamp
	s.TsLastSeq++
}

func TsFileAppendKeyFrame(s *Stream, c *Chunk) {
	pesHeader, pesHeaderData := PesHeaderCreate(s, c)
	pesData := PesDataCreateKeyFrame(s, c, pesHeaderData)

	var pesDataLen = len(pesData)
	//SetPesPakcetLength(pesData, uint16(pesDataLen)-6)

	var consumeLen, start int
	// pcr 和 dts是什么关系???
	s.TsData, consumeLen = TsPacketCreateKeyFrame(s, VideoPid, pesData[start:], pesHeader.Dts)
	start += consumeLen

	_, err := s.TsFile.Write(s.TsData)
	if err != nil {
		s.logHls.Printf("Write ts fail, %s", err)
		return
	}

	for {
		s.TsData, consumeLen = TsPacketCreate(s, VideoPid, pesData[start:])
		start += consumeLen

		_, err := s.TsFile.Write(s.TsData)
		if err != nil {
			s.logHls.Printf("Write ts fail, %s", err)
			return
		}

		if start >= pesDataLen {
			s.logHls.Printf("pesDataLen=%d, start=%d", pesDataLen, start)
			break
		}
	}
}

func TsFileAppendInterFrame(s *Stream, c *Chunk) {
	_, pesHeaderData := PesHeaderCreate(s, c)
	pesData := PesDataCreateInterFrame(s, c, pesHeaderData)

	var pesDataLen = len(pesData)
	SetPesPakcetLength(pesData, uint16(pesDataLen)-6)

	var consumeLen, start int
	s.TsData, consumeLen = TsPacketCreateInterFrame(s, VideoPid, pesData[start:])
	start += consumeLen

	_, err := s.TsFile.Write(s.TsData)
	if err != nil {
		s.logHls.Printf("Write ts fail, %s", err)
		return
	}

	for {
		s.TsData, consumeLen = TsPacketCreate(s, VideoPid, pesData[start:])
		start += consumeLen

		_, err := s.TsFile.Write(s.TsData)
		if err != nil {
			s.logHls.Printf("Write ts fail, %s", err)
			return
		}

		if start >= pesDataLen {
			s.logHls.Printf("pesDataLen=%d, start=%d", pesDataLen, start)
			break
		}
	}
}

func TsFileAppendAacFrame(s *Stream, c *Chunk) {
	_, pesHeaderData := PesHeaderCreate(s, c)
	pesData := PesDataCreateAacFrame(s, c, pesHeaderData)
	//s.logHls.Printf(">>> %x", pesData)

	var pesDataLen = len(pesData)
	SetPesPakcetLength(pesData, uint16(pesDataLen)-6)

	var consumeLen, start int
	s.TsData, consumeLen = TsPacketCreateAacFrame(s, AudioPid, pesData[start:])
	start += consumeLen

	//s.logHls.Printf(">>> %x", s.TsData)
	_, err := s.TsFile.Write(s.TsData)
	if err != nil {
		s.logHls.Printf("Write ts fail, %s", err)
		return
	}

	for {
		s.TsData, consumeLen = TsPacketCreate(s, AudioPid, pesData[start:])
		start += consumeLen

		//s.logHls.Printf(">>> %x", s.TsData)
		_, err := s.TsFile.Write(s.TsData)
		if err != nil {
			s.logHls.Printf("Write ts fail, %s", err)
			return
		}

		if start >= pesDataLen {
			s.logHls.Printf("pesDataLen=%d, start=%d", pesDataLen, start)
			break
		}
	}
}

func TsFileAppend(s *Stream, c *Chunk) {
	switch c.DataType {
	case "VideoKeyFrame":
		TsFileAppendKeyFrame(s, c)
	case "VideoInterFrame":
		TsFileAppendInterFrame(s, c)
	case "AudioAacFrame":
		TsFileAppendAacFrame(s, c)
	default:
	}
}

// 新生成一个ts返回true, 否则返回false
func TsCreate(s *Stream, c *Chunk) bool {
	// rtmp里的timestamp单位是毫秒, 除以1000变为秒
	s.TsExtInfo = float64(c.Timestamp-s.TsFirstTs) / 1000
	s.logHls.Printf("c.Timestamp=%d, s.TsFirstTs=%d, s.TsExtInfo=%f, conf.HlsTsMaxTime=%d", c.Timestamp, s.TsFirstTs, s.TsExtInfo, conf.HlsTsMaxTime)

	var tf bool
	if s.TsPath == "" || (uint32(s.TsExtInfo) >= conf.HlsTsMaxTime && c.DataType == "VideoKeyFrame") {
		s.logHls.Println("--->> TsFileCreate()")
		TsFileCreate(s, c) // 新建TsFile, 并写入
		tf = true
	} else {
		s.logHls.Println("--->> TsFileAppend()")
		TsFileAppend(s, c) // 可以写入当前TsFile
		tf = false
	}
	return tf
}

/**********************************************************/
/* pes
/**********************************************************/
// PTS or DTS
// 4 + 3 + 1 + 15 + 1 + 15 + 1 = 5byte
type OptionalTs struct {
	FixedValue1 uint8  // 4bit, PTS:0x0010 or 0x0011, DTS:0x0001
	Ts32_30     uint8  // 3bit, 33bit
	MarkerBit0  uint8  // 1bit
	Ts29_15     uint16 // 15bit
	MarkerBit1  uint8  // 1bit
	Ts14_0      uint16 // 15bit
	MarkerBit2  uint8  // 1bit
}

// PtsDtsFlags            uint8  // 2bit
// 0x0 00, 没有PTS和DTS
// 0x1 01, 禁止使用
// 0x2 10, 只有PTS
// 0x3 11, 有PTS 有DTS
// 1 + 1 + 1 = 3byte
type OptionalPesHeader struct {
	FixedValue0            uint8 // 2bit, 固定值0x2
	PesScramblingControl   uint8 // 2bit, 加扰控制
	PesPriority            uint8 // 1bit, 优先级
	DataAlignmentIndicator uint8 // 1bit,
	Copyright              uint8 // 1bit
	OriginalOrCopy         uint8 // 1bit, 原始或复制
	PtsDtsFlags            uint8 // 2bit, 时间戳标志位, 00表示没有对应的信息; 01是被禁用的; 10表示只有PTS; 11表示有PTS和DTS
	EscrFlag               uint8 // 1bit
	EsRateFlag             uint8 // 1bit, 对于PES流而言，它指出了系统目标解码器接收PES分组的速率。该字段在它所属的PES分组以及同一个PES流的后续PES分组中一直有效，直到遇到一个新的ES_rate字段。该字段的值以50字节/秒为单位，且不能为0。
	DsmTrickModeFlag       uint8 // 1bit, 表示作用于相关视频流的特技方式(快进/快退/冻结帧)
	AdditionalCopyInfoFlag uint8 // 1bit
	PesCrcFlag             uint8 // 1bit
	PesExtensionFlag       uint8 // 1bit
	PesHeaderDataLength    uint8 // 8bit, 表示后面还有x个字节, 之后就是负载数据
}

// 6 + 3 + 5 = 14byte
// 6 + 3 + 5 + 5 = 19byte
type PesHeader struct {
	PacketStartCodePrefix uint32 // 24bit, 固定值 0x000001
	StreamId              uint8  // 8bit, 0xe0视频 0xc0音频
	PesPacketLength       uint16 // 16bit, 包长度, 表示后面还有x个字节的数据，包括剩余的pes头数据和负载数据, 最大值65536
	OptionalPesHeader
	OptionalTs        //oTs []OptionalTs
	Pts        uint64 // 33bit, 不是包结构成员, 只是方便编码
	Dts        uint64 // 33bit, 不是包结构成员, 只是方便编码
}

// rtmp里面的数据 是ES(h264/aac), tsFile里是PES
// rtmp的message(chunk)转换为pes
// rtmp里的Timestamp应该是dts
// 一个pes就是一帧数据(关键帧/非关键帧/音频帧)
// PTS 和 DTS
// GOP分为开放式和闭合式, 最后一帧不是P帧为开放式, 最后一帧是P帧为闭合式; GOP中 不能没有I帧，不能没有P帧，可以没有B帧(如监控视频);
// 音频的pts等于dts; 视频I帧(关键帧)的pts等于dts;
// 视频P帧(没有B帧)的pts等于dts; 视频P帧(有B帧)的pts不等于dts;
// 视频B帧(没有P帧)的pts等于dts; 视频B帧(有P帧)的pts不等于dts;
func PesHeaderCreate(s *Stream, c *Chunk) (*PesHeader, []byte) {
	dts := uint64(c.Timestamp * H264ClockFrequency)
	pts := dts
	var CompositionTime uint32
	if c.MsgTypeId == MsgTypeIdVideo { // 9
		CompositionTime = ByteToUint32(c.MsgData[2:5], BE) // 24bit
		pts = dts + uint64(CompositionTime*H264ClockFrequency)
	}
	s.logHls.Printf("c.DataType=%s, pts=%d, dts=%d, CompositionTime=%d", c.DataType, pts, dts, CompositionTime)

	var pes PesHeader
	pes.PacketStartCodePrefix = 0x000001
	if c.MsgTypeId == MsgTypeIdAudio { // 8
		pes.StreamId = AudioStreamId
	}
	if c.MsgTypeId == MsgTypeIdVideo { // 9
		pes.StreamId = VideoStreamId
	}
	pes.PtsDtsFlags = 0x2 // 只有PTS, 40bit
	pes.PesPacketLength = 3 + 5
	pes.PesHeaderDataLength = 5
	if pts != dts {
		pes.PtsDtsFlags = 0x3 // 有PTS 有DTS, 40bit + 40bit
		pes.PesPacketLength = 3 + 10
		pes.PesHeaderDataLength = 10
	}

	//var oPesHeader OptionalPesHeader
	if pes.PesHeaderDataLength != 0 {
		pes.FixedValue0 = 0x2
		pes.PesScramblingControl = 0x0
		pes.PesPriority = 0x0
		pes.DataAlignmentIndicator = 0x0
		pes.Copyright = 0x0
		pes.OriginalOrCopy = 0x0
		//pes.PtsDtsFlags = 0
		pes.EscrFlag = 0x0
		pes.EsRateFlag = 0x0
		pes.DsmTrickModeFlag = 0x0
		pes.AdditionalCopyInfoFlag = 0x0
		pes.PesCrcFlag = 0x0
		pes.PesExtensionFlag = 0x0
		//pes.PesHeaderDataLength
	}

	pes.Pts = pts
	pes.Dts = dts
	//s.logHls.Printf("%#v", pes)

	pesLen := 6 + pes.PesPacketLength
	pesData := make([]byte, pesLen)

	Uint24ToByte(pes.PacketStartCodePrefix, pesData[0:3], BE)
	pesData[3] = pes.StreamId
	// 这里还不知道数据的长度，所以先赋值为0
	// 后续用 SetPesPakcetLength() 重新赋值
	pes.PesPacketLength = 0x0
	Uint16ToByte(pes.PesPacketLength, pesData[4:6], BE)
	pesData[6] = (pes.FixedValue0&0x3)<<6 | (pes.PesScramblingControl&0x3)<<4 | (pes.PesPriority&0x1)<<3 | (pes.DataAlignmentIndicator&0x1)<<2 | (pes.Copyright&0x1)<<1 | (pes.OriginalOrCopy & 0x1)
	pesData[7] = (pes.PtsDtsFlags&0x3)<<6 | (pes.EscrFlag&0x1)<<5 | (pes.EsRateFlag&0x1)<<4 | (pes.DsmTrickModeFlag&0x1)<<3 | (pes.AdditionalCopyInfoFlag&0x1)<<2 | (pes.PesCrcFlag&0x1)<<1 | (pes.PesExtensionFlag & 0x1)
	pesData[8] = pes.PesHeaderDataLength

	pes.MarkerBit0 = 0x1
	pes.MarkerBit1 = 0x1
	pes.MarkerBit2 = 0x1
	if pes.PtsDtsFlags == 0x2 { // 只有PTS, 40bit
		pes.FixedValue1 = 0x2
		pesData[9] = (pes.FixedValue1&0xf)<<4 | uint8((pes.Pts>>29)&0xd) | (pes.MarkerBit0 & 0x1)
		pesData[10] = uint8((pes.Pts >> 22) & 0xff)
		pesData[11] = uint8((pes.Pts>>14)&0xfe) | (pes.MarkerBit1 & 0x1)
		pesData[12] = uint8((pes.Pts >> 7) & 0xff)
		pesData[13] = uint8((pes.Pts&0x7F)<<1) | (pes.MarkerBit2 & 0x1)
	}
	if pes.PtsDtsFlags == 0x3 { // 有PTS 有DTS, 40bit + 40bit
		pes.FixedValue1 = 0x3
		pesData[9] = (pes.FixedValue1&0xf)<<4 | uint8((pes.Pts>>29)&0xd) | (pes.MarkerBit0 & 0x1)
		pesData[10] = uint8((pes.Pts >> 22) & 0xff)
		pesData[11] = uint8((pes.Pts>>14)&0xfe) | (pes.MarkerBit1 & 0x1)
		pesData[12] = uint8((pes.Pts >> 7) & 0xff)
		pesData[13] = uint8((pes.Pts&0x7F)<<1) | (pes.MarkerBit2 & 0x1)
		pes.FixedValue1 = 0x1
		pesData[14] = (pes.FixedValue1&0xf)<<4 | uint8((pes.Dts>>29)&0xd) | (pes.MarkerBit0 & 0x1)
		pesData[15] = uint8((pes.Dts >> 22) & 0xff)
		pesData[16] = uint8((pes.Dts>>14)&0xfe) | (pes.MarkerBit1 & 0x1)
		pesData[17] = uint8((pes.Dts >> 7) & 0xff)
		pesData[18] = uint8((pes.Dts&0x7F)<<1) | (pes.MarkerBit2 & 0x1)
	}
	s.logHls.Printf("%#v", pes)
	return &pes, pesData
}

func SetPesPakcetLength(d []byte, size uint16) {
	// 16bit, 最大值65536, 如果放不下就不放了
	if size > 0xffff {
		return
	}
	Uint16ToByte(size, d[4:6], BE)
}

//0x00000001 + 0x09 + 0xf0, ffmpeg转出的ts有这6个字节, 没有也可以
//0x00000001 + 0x67 + sps + 0x00000001 + 0x68 + pps + 0x00000001 + 0x65 + iFrame
// 返回值: pesHeader + pesBody
func PesDataCreateKeyFrame(s *Stream, c *Chunk, phd []byte) []byte {
	pesHeaderDataLen := len(phd)
	SpsPpsDataLen := len(s.SpsPpsData)
	MsgDataLen := int(c.MsgLength) - 9
	dataLen := pesHeaderDataLen + SpsPpsDataLen + 6 + 3 + MsgDataLen
	s.logHls.Println(pesHeaderDataLen, SpsPpsDataLen, 9, MsgDataLen, dataLen)
	data := make([]byte, dataLen)

	ss := 0
	ee := pesHeaderDataLen
	copy(data[ss:ee], phd)
	ss = ee
	ee += 4
	Uint32ToByte(0x00000001, data[ss:ee], BE)
	ss = ee
	ee += 1
	data[ss] = 0x09
	ss = ee
	ee += 1
	data[ss] = 0xf0
	ss = ee
	ee += SpsPpsDataLen
	copy(data[ss:ee], s.SpsPpsData)
	ss = ee
	ee += 3
	Uint24ToByte(0x000001, data[ss:ee], BE)
	//Uint32ToByte(0x00000001, data[ss:ee], BE)
	ss = ee
	ee += MsgDataLen
	//s.logHls.Printf("%x", c.MsgData)
	copy(data[ss:], c.MsgData[9:])
	return data
}

func PesDataCreateInterFrame(s *Stream, c *Chunk, phd []byte) []byte {
	pesHeaderDataLen := len(phd)
	MsgDataLen := int(c.MsgLength) - 9
	dataLen := pesHeaderDataLen + 6 + 3 + MsgDataLen
	data := make([]byte, dataLen)

	ss := 0
	ee := pesHeaderDataLen
	copy(data[ss:ee], phd)
	ss = ee
	ee += 4
	Uint32ToByte(0x00000001, data[ss:ee], BE)
	ss = ee
	ee += 1
	data[ss] = 0x09
	ss = ee
	ee += 1
	data[ss] = 0xf0
	ss = ee
	ee += 3
	Uint24ToByte(0x000001, data[ss:ee], BE)
	//Uint32ToByte(0x00000001, data[ss:ee], BE)
	ss = ee
	ee += MsgDataLen
	copy(data[ss:], c.MsgData[9:])
	return data
}

func PesDataCreateAacFrame(s *Stream, c *Chunk, phd []byte) []byte {
	pesHeaderDataLen := len(phd)
	MsgDataLen := int(c.MsgLength) - 2
	dataLen := pesHeaderDataLen + 7 + MsgDataLen
	data := make([]byte, dataLen)

	ss := 0
	ee := pesHeaderDataLen
	copy(data[ss:ee], phd)
	ss = ee
	ee += 7
	// 函数A 把[]byte 传给函数B, B修改后 A里的值也会变
	SetAdtsLength(s.AdtsData, uint16(7+MsgDataLen))
	ParseAdtsData(s)
	copy(data[ss:ee], s.AdtsData)
	ss = ee
	ee += MsgDataLen
	copy(data[ss:], c.MsgData[2:])
	//s.logHls.Printf("%x", data)
	return data
}

/**********************************************************/
/* pat
/**********************************************************/
type PatProgram struct {
	ProgramNumber uint16 // 16bit, arr 4byte,  0 is NetworkPid
	Reserved2     uint8  // 3bit, arr
	PID           uint16 // 13bit, arr, NetworkPid or ProgramMapPid
}

// 3 + 5 + 4 + 4 = 16byte
type Pat struct {
	TableId                uint8  // 8bit, 固定值0x00, 表示是PAT
	SectionSyntaxIndicator uint8  // 1bit, 固定值0x1
	Zero                   uint8  // 1bit, 0x0
	Reserved0              uint8  // 2bit, 0x3
	SectionLength          uint16 // 12bit, 表示后面还有多少字节 包括CRC32
	TransportStreamId      uint16 // 16bit, 传输流id, 区别与其他路流id
	Reserved1              uint8  // 2bit, 保留位
	VersionNumber          uint8  // 5bit, 范围0-31，表示PAT的版本号
	CurrentNextIndicator   uint8  // 1bit, 是当前有效还是下一个有效
	SectionNumber          uint8  // 8bit, PAT可能分为多段传输，第一段为00，以后每个分段加1，最多可能有256个分段
	LastSectionNumber      uint8  // 8bit, 最后一个分段的号码
	ProgramNumber          uint16 // 16bit, arr 4byte,  0 is NetworkPid
	Reserved2              uint8  // 3bit, arr
	PID                    uint16 // 13bit, arr, NetworkPid or ProgramMapPid
	CRC32                  uint32 // 32bit
}

func PatCreate() (*Pat, []byte) {
	var pat Pat
	pat.TableId = 0x00
	pat.SectionSyntaxIndicator = 0x1
	pat.Zero = 0x0
	pat.Reserved0 = 0x3
	pat.SectionLength = 0xd // 13 = 5 + 4 + 4
	pat.TransportStreamId = 0x1
	pat.Reserved1 = 0x3
	pat.VersionNumber = 0x0
	pat.CurrentNextIndicator = 0x1
	pat.SectionNumber = 0x0
	pat.LastSectionNumber = 0x0
	pat.ProgramNumber = 0x1
	pat.Reserved2 = 0x7
	pat.PID = PmtPid
	pat.CRC32 = 0

	patData := make([]byte, 16)
	patData[0] = pat.TableId
	patData[1] = (pat.SectionSyntaxIndicator&0x1)<<7 | (pat.Zero&0x1)<<6 | (pat.Reserved0&0x3)<<4 | uint8((pat.SectionLength&0xf00)>>8)
	patData[2] = uint8(pat.SectionLength & 0xff)
	Uint16ToByte(pat.TransportStreamId, patData[3:5], BE)
	patData[5] = (pat.Reserved1&0x3)<<6 | (pat.VersionNumber&0x1f)<<1 | (pat.CurrentNextIndicator & 0x1)
	patData[6] = pat.SectionNumber
	patData[7] = pat.LastSectionNumber
	Uint16ToByte(pat.ProgramNumber, patData[8:10], BE)
	patData[10] = (pat.Reserved2&0x7)<<5 | uint8((pat.PID&0x1f00)>>8)
	patData[11] = uint8(pat.PID & 0xff)

	pat.CRC32 = Crc32Create(patData[:12])
	Uint32ToByte(pat.CRC32, patData[12:16], BE)
	return &pat, patData
}

/**********************************************************/
/* pmt
/**********************************************************/
// StreamType             uint8  // 8bit, arr 5byte
// 0x0f		Audio with ADTS transport syntax
// 0x1b		H.264
// 40bit = 5byte
type PmtStream struct {
	StreamType    uint8  // 8bit, 节目数据类型
	Reserved4     uint8  // 3bit,
	ElementaryPID uint16 // 13bit, 节目数据类型对应的pid
	Reserved5     uint8  // 4bit,
	EsInfoLength  uint16 // 12bit, 私有数据长度
}

// 3 + 9 + 5*2 + 4 = 26byte
type Pmt struct {
	TableId                uint8       // 8bit, 固定值0x02, 表示是PMT
	SectionSyntaxIndicator uint8       // 1bit, 固定值0x1
	Zero                   uint8       // 1bit, 固定值0x0
	Reserved0              uint8       // 2bit, 0x3
	SectionLength          uint16      // 12bit, 表示后面还有多少字节 包括CRC32
	ProgramNumber          uint16      // 16bit, 不同节目此值不同 依次递增
	Reserved1              uint8       // 2bit, 0x3
	VersionNumber          uint8       // 5bit, 指示当前TS流中program_map_secton 的版本号
	CurrentNextIndicator   uint8       // 1bit, 当该字段为1时表示当前传送的program_map_section可用，当该字段为0时，表示当前传送的program_map_section不可用，下一个TS的program_map_section有效。
	SectionNumber          uint8       // 8bit, 0x0
	LastSectionNumber      uint8       // 8bit, 0x0
	Reserved2              uint8       // 3bit, 0x7
	PcrPID                 uint16      // 13bit, pcr会在哪个pid包里出现，一般是视频包里，PcrPID设置为 0x1fff 表示没有pcr
	Reserved3              uint8       // 4bit, 0xf
	ProgramInfoLength      uint16      // 12bit, 节目信息描述的字节数, 通常为 0x0
	PmtStream              []PmtStream // 40bit, 节目信息
	CRC32                  uint32      // 32bit
}

func PmtCreate() (*Pmt, []byte) {
	var pmt Pmt
	pmt.TableId = 0x2
	pmt.SectionSyntaxIndicator = 0x1
	pmt.Zero = 0x0
	pmt.Reserved0 = 0x3
	pmt.SectionLength = 0x17
	pmt.ProgramNumber = 0x1
	pmt.Reserved1 = 0x3
	pmt.VersionNumber = 0x0
	pmt.CurrentNextIndicator = 0x1
	pmt.SectionNumber = 0x0
	pmt.LastSectionNumber = 0x0
	pmt.Reserved2 = 0x7
	pmt.PcrPID = VideoPid
	pmt.Reserved3 = 0xf
	pmt.ProgramInfoLength = 0x0
	pmt.PmtStream = make([]PmtStream, 2)
	pmt.PmtStream[1].StreamType = 0x1b // AVC video stream as defined in ITU-T Rec. H.264 | ISO/IEC 14496-10 Video
	pmt.PmtStream[1].Reserved4 = 0x7
	pmt.PmtStream[1].ElementaryPID = VideoPid
	pmt.PmtStream[1].Reserved5 = 0xf
	pmt.PmtStream[1].EsInfoLength = 0x0
	pmt.PmtStream[0].StreamType = 0xf // ISO/IEC 13818-7 Audio with ADTS transport syntax
	pmt.PmtStream[0].Reserved4 = 0x7
	pmt.PmtStream[0].ElementaryPID = AudioPid
	pmt.PmtStream[0].Reserved5 = 0xf
	pmt.PmtStream[0].EsInfoLength = 0x0
	pmt.CRC32 = 0

	pmtData := make([]byte, 26)
	pmtData[0] = pmt.TableId
	pmtData[1] = (pmt.SectionSyntaxIndicator&0x1)<<7 | (pmt.Zero&0x1)<<6 | (pmt.Reserved0&0x3)<<4 | uint8((pmt.SectionLength&0xf00)>>8)
	pmtData[2] = uint8(pmt.SectionLength & 0xff)
	Uint16ToByte(pmt.ProgramNumber, pmtData[3:5], BE)
	pmtData[5] = (pmt.Reserved1&0x3)<<6 | (pmt.VersionNumber&0x1f)<<1 | (pmt.CurrentNextIndicator & 0x1)
	pmtData[6] = pmt.SectionNumber
	pmtData[7] = pmt.LastSectionNumber
	pmtData[8] = (pmt.Reserved2&0x7)<<5 | uint8((pmt.PcrPID&0x1f00)>>8)
	pmtData[9] = uint8(pmt.PcrPID & 0xff)
	pmtData[10] = (pmt.Reserved3&0xf)<<4 | uint8((pmt.ProgramInfoLength&0xf00)>>8)
	pmtData[11] = uint8(pmt.ProgramInfoLength & 0xff)
	ps0 := pmt.PmtStream[0]
	ps1 := pmt.PmtStream[1]
	pmtData[12] = ps0.StreamType
	pmtData[13] = (ps0.Reserved4&0x7)<<5 | uint8((ps0.ElementaryPID&0x1f00)>>8)
	pmtData[14] = uint8(ps0.ElementaryPID & 0xff)
	pmtData[15] = (ps0.Reserved5|0xf)<<4 | uint8((ps0.EsInfoLength&0xf00)>>8)
	pmtData[16] = uint8(ps0.EsInfoLength & 0xff)
	pmtData[17] = ps1.StreamType
	pmtData[18] = (ps1.Reserved4&0x7)<<5 | uint8((ps1.ElementaryPID&0x1f00)>>8)
	pmtData[19] = uint8(ps1.ElementaryPID & 0xff)
	pmtData[20] = (ps1.Reserved5|0xf)<<4 | uint8((ps1.EsInfoLength&0xf00)>>8)
	pmtData[21] = uint8(ps1.EsInfoLength & 0xff)

	pmt.CRC32 = Crc32Create(pmtData[:22])
	Uint32ToByte(pmt.CRC32, pmtData[22:26], BE)
	return &pmt, pmtData
}

/**********************************************************/
/* crc
/**********************************************************/
var crcTable = []uint32{
	0x00000000, 0x04c11db7, 0x09823b6e, 0x0d4326d9,
	0x130476dc, 0x17c56b6b, 0x1a864db2, 0x1e475005,
	0x2608edb8, 0x22c9f00f, 0x2f8ad6d6, 0x2b4bcb61,
	0x350c9b64, 0x31cd86d3, 0x3c8ea00a, 0x384fbdbd,
	0x4c11db70, 0x48d0c6c7, 0x4593e01e, 0x4152fda9,
	0x5f15adac, 0x5bd4b01b, 0x569796c2, 0x52568b75,
	0x6a1936c8, 0x6ed82b7f, 0x639b0da6, 0x675a1011,
	0x791d4014, 0x7ddc5da3, 0x709f7b7a, 0x745e66cd,
	0x9823b6e0, 0x9ce2ab57, 0x91a18d8e, 0x95609039,
	0x8b27c03c, 0x8fe6dd8b, 0x82a5fb52, 0x8664e6e5,
	0xbe2b5b58, 0xbaea46ef, 0xb7a96036, 0xb3687d81,
	0xad2f2d84, 0xa9ee3033, 0xa4ad16ea, 0xa06c0b5d,
	0xd4326d90, 0xd0f37027, 0xddb056fe, 0xd9714b49,
	0xc7361b4c, 0xc3f706fb, 0xceb42022, 0xca753d95,
	0xf23a8028, 0xf6fb9d9f, 0xfbb8bb46, 0xff79a6f1,
	0xe13ef6f4, 0xe5ffeb43, 0xe8bccd9a, 0xec7dd02d,
	0x34867077, 0x30476dc0, 0x3d044b19, 0x39c556ae,
	0x278206ab, 0x23431b1c, 0x2e003dc5, 0x2ac12072,
	0x128e9dcf, 0x164f8078, 0x1b0ca6a1, 0x1fcdbb16,
	0x018aeb13, 0x054bf6a4, 0x0808d07d, 0x0cc9cdca,
	0x7897ab07, 0x7c56b6b0, 0x71159069, 0x75d48dde,
	0x6b93dddb, 0x6f52c06c, 0x6211e6b5, 0x66d0fb02,
	0x5e9f46bf, 0x5a5e5b08, 0x571d7dd1, 0x53dc6066,
	0x4d9b3063, 0x495a2dd4, 0x44190b0d, 0x40d816ba,
	0xaca5c697, 0xa864db20, 0xa527fdf9, 0xa1e6e04e,
	0xbfa1b04b, 0xbb60adfc, 0xb6238b25, 0xb2e29692,
	0x8aad2b2f, 0x8e6c3698, 0x832f1041, 0x87ee0df6,
	0x99a95df3, 0x9d684044, 0x902b669d, 0x94ea7b2a,
	0xe0b41de7, 0xe4750050, 0xe9362689, 0xedf73b3e,
	0xf3b06b3b, 0xf771768c, 0xfa325055, 0xfef34de2,
	0xc6bcf05f, 0xc27dede8, 0xcf3ecb31, 0xcbffd686,
	0xd5b88683, 0xd1799b34, 0xdc3abded, 0xd8fba05a,
	0x690ce0ee, 0x6dcdfd59, 0x608edb80, 0x644fc637,
	0x7a089632, 0x7ec98b85, 0x738aad5c, 0x774bb0eb,
	0x4f040d56, 0x4bc510e1, 0x46863638, 0x42472b8f,
	0x5c007b8a, 0x58c1663d, 0x558240e4, 0x51435d53,
	0x251d3b9e, 0x21dc2629, 0x2c9f00f0, 0x285e1d47,
	0x36194d42, 0x32d850f5, 0x3f9b762c, 0x3b5a6b9b,
	0x0315d626, 0x07d4cb91, 0x0a97ed48, 0x0e56f0ff,
	0x1011a0fa, 0x14d0bd4d, 0x19939b94, 0x1d528623,
	0xf12f560e, 0xf5ee4bb9, 0xf8ad6d60, 0xfc6c70d7,
	0xe22b20d2, 0xe6ea3d65, 0xeba91bbc, 0xef68060b,
	0xd727bbb6, 0xd3e6a601, 0xdea580d8, 0xda649d6f,
	0xc423cd6a, 0xc0e2d0dd, 0xcda1f604, 0xc960ebb3,
	0xbd3e8d7e, 0xb9ff90c9, 0xb4bcb610, 0xb07daba7,
	0xae3afba2, 0xaafbe615, 0xa7b8c0cc, 0xa379dd7b,
	0x9b3660c6, 0x9ff77d71, 0x92b45ba8, 0x9675461f,
	0x8832161a, 0x8cf30bad, 0x81b02d74, 0x857130c3,
	0x5d8a9099, 0x594b8d2e, 0x5408abf7, 0x50c9b640,
	0x4e8ee645, 0x4a4ffbf2, 0x470cdd2b, 0x43cdc09c,
	0x7b827d21, 0x7f436096, 0x7200464f, 0x76c15bf8,
	0x68860bfd, 0x6c47164a, 0x61043093, 0x65c52d24,
	0x119b4be9, 0x155a565e, 0x18197087, 0x1cd86d30,
	0x029f3d35, 0x065e2082, 0x0b1d065b, 0x0fdc1bec,
	0x3793a651, 0x3352bbe6, 0x3e119d3f, 0x3ad08088,
	0x2497d08d, 0x2056cd3a, 0x2d15ebe3, 0x29d4f654,
	0xc5a92679, 0xc1683bce, 0xcc2b1d17, 0xc8ea00a0,
	0xd6ad50a5, 0xd26c4d12, 0xdf2f6bcb, 0xdbee767c,
	0xe3a1cbc1, 0xe760d676, 0xea23f0af, 0xeee2ed18,
	0xf0a5bd1d, 0xf464a0aa, 0xf9278673, 0xfde69bc4,
	0x89b8fd09, 0x8d79e0be, 0x803ac667, 0x84fbdbd0,
	0x9abc8bd5, 0x9e7d9662, 0x933eb0bb, 0x97ffad0c,
	0xafb010b1, 0xab710d06, 0xa6322bdf, 0xa2f33668,
	0xbcb4666d, 0xb8757bda, 0xb5365d03, 0xb1f740b4,
}

func Crc32Create(src []byte) uint32 {
	crc32 := uint32(0xFFFFFFFF)
	j := byte(0)
	for i := 0; i < len(src); i++ {
		j = (byte(crc32>>24) ^ src[i]) & 0xff
		crc32 = uint32(uint32(crc32<<8) ^ uint32(crcTable[j]))
	}
	return crc32
}

/**********************************************************/
/* m3u8
/**********************************************************/
// hls协议规范
//https://datatracker.ietf.org/doc/html/draft-pantos-http-live-streaming-08  这个各个版本都有
//https://www.rfc-editor.org/rfc/rfc8216.html  这个只有最新版

//#EXT-X-PLAYLIST-TYPE:<type-enum>
//where type-enum is either EVENT or VOD.
//A Live Playlist MUST NOT contain the EXT-X-PLAYLIST-TYPE tag, as no value of that tag allows Media Segments to be removed.
//#EXT-X-VERSION标签大于等于3时, #EXTINF时长可以为小数

/*
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:1
#EXTINF:10,
live_cctv1_h264_aac_timestamp_1.ts
#EXTINF:10,
live_cctv1_h264_aac_timestamp_2.ts
#EXTINF:10,
live_cctv1_h264_aac_timestamp_3.ts
*/

var m3u8Head = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:%d
#EXT-X-MEDIA-SEQUENCE:%d`

//#EXT-X-DISCONTINUITY
var m3u8Body = `#EXTINF:%.2f, no desc
%s`

type TsInfo struct {
	TsInfoStr  string  // m3u8里ts的记录
	TsExtInfo  float64 // ts文件的播放时长
	TsFilepath string  // ts存储路径 包含文件名
}

func M3u8Update(s *Stream, c *Chunk) {
	// s.TsNum 初始值为0, conf.HlsM3u8TsNum 通常为6
	if s.TsNum == uint32(conf.HlsM3u8TsNum) {
		e := s.TsList.Front()
		ti := (e.Value).(TsInfo)
		os.Remove(ti.TsFilepath)
		s.TsList.Remove(e)
		s.TsNum--
		s.TsFirstSeq++
	}
	tiStr := fmt.Sprintf(m3u8Body, s.TsExtInfo, path.Base(s.TsPath))
	ti := TsInfo{tiStr, s.TsExtInfo, s.TsPath}
	s.TsList.PushBack(ti)
	s.TsNum++

	var tsMaxTime float64
	var tis string
	for e := s.TsList.Front(); e != nil; e = e.Next() {
		ti = (e.Value).(TsInfo)
		if tsMaxTime < ti.TsExtInfo {
			tsMaxTime = ti.TsExtInfo
		}
		tis = fmt.Sprintf("%s\n%s", tis, ti.TsInfoStr)
	}

	s.M3u8Data = fmt.Sprintf(m3u8Head, uint32(math.Ceil(tsMaxTime)), s.TsFirstSeq)
	s.M3u8Data = fmt.Sprintf("%s%s", s.M3u8Data, tis)
	//s.logHls.Println(s.M3u8Data)

	// 打开文件
	var err error
	if s.M3u8File == nil {
		s.M3u8File, err = os.OpenFile(s.M3u8Path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			log.Println(err)
			return
		}
	}
	// 写入文件
	_, err = s.M3u8File.WriteString(s.M3u8Data)
	if err != nil {
		s.logHls.Printf("Write %s fail, %s", s.M3u8Path, err)
		return
	}
	// 关闭文件
	err = s.M3u8File.Close()
	if err != nil {
		log.Println(err)
		return
	}
	s.M3u8File = nil
}

func M3u8Update0(s *Stream, c *Chunk) {
	// s.TsNum 初始值为0, conf.HlsM3u8TsNum 通常为6
	if s.TsNum == uint32(conf.HlsM3u8TsNum) {
		e := s.TsList.Front()
		ti := (e.Value).(TsInfo)
		os.Remove(ti.TsFilepath)
		s.TsList.Remove(e)
		s.TsNum--
		s.TsFirstSeq++
	}
	tiStr := fmt.Sprintf(m3u8Body, s.TsExtInfo, path.Base(s.TsPath))
	ti := TsInfo{tiStr, s.TsExtInfo, s.TsPath}
	s.TsList.PushBack(ti)
	s.TsNum++

	s.M3u8Data = fmt.Sprintf(m3u8Head, conf.HlsTsMaxTime, s.TsFirstSeq)

	var tis string
	for e := s.TsList.Front(); e != nil; e = e.Next() {
		ti = (e.Value).(TsInfo)
		tis = fmt.Sprintf("%s\n%s", tis, ti.TsInfoStr)
	}
	s.M3u8Data = fmt.Sprintf("%s%s", s.M3u8Data, tis)
	//s.logHls.Println(s.M3u8Data)

	// 清空文件
	err := s.M3u8File.Truncate(0)
	if err != nil {
		s.logHls.Println(err)
		return
	}
	_, err = s.M3u8File.Seek(0, 0)
	if err != nil {
		s.logHls.Println(err)
		return
	}
	// 写入文件
	_, err = s.M3u8File.WriteString(s.M3u8Data)
	if err != nil {
		s.logHls.Printf("Write %s fail, %s", s.M3u8Path, err)
		return
	}
}

/**********************************************************/
/* http
/**********************************************************/
// rtmp://127.0.0.1/live/yuankang
// http://127.0.0.1/live/yuankang.flv
// http://127.0.0.1/live/yuankang.m3u8
// live_yuankang/hls/live_yuankang.m3u8
// http://127.0.0.1/live/live_yuankang_0.ts
// live_yuankang/hls/live_yuankang_0.ts

// 返回 app, stream, filename
func GetPlayInfo(url string) (string, string, string) {
	ext := path.Ext(url)
	switch ext {
	case ".m3u8", ".flv":
		s := strings.Split(url, "/")
		if len(s) < 3 {
			return "", "", ""
		}
		ss := strings.Split(s[2], ".")
		if len(ss) < 1 {
			return "", "", ""
		}
		return s[1], ss[0], path.Base(url)
	case ".ts":
		//dir := path.Dir(url) // /live
		fn := path.Base(url) // live_yuankang_0.ts
		s := strings.Split(fn, "_")
		if len(s) < 3 {
			return "", "", ""
		}
		return s[0], s[1], fn
	}
	return "", "", ""
}

func GetM3u8(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	//app, stream, fn := GetPlayInfo(r.URL.String())
	app, stream, _ := GetPlayInfo(r.URL.String())
	file := fmt.Sprintf("%s%s_%s/%s_%s.m3u8", conf.HlsSavePath, app, stream, app, stream)
	//log.Println(app, stream, fn, file)

	d, err := utils.ReadAllFile(file)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return d, nil
}

func GetTs(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	app, stream, fn := GetPlayInfo(r.URL.String())
	file := fmt.Sprintf("%s%s_%s/%s", conf.HlsSavePath, app, stream, fn)
	//log.Println(app, stream, fn, file)

	d, err := utils.ReadAllFile(file)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return d, nil
}

// 四舍五入取整
// 1.4 + 0.5 = 1.9 向下取整为 1
// 1.5 + 0.5 = 2.0 向下取整为 2
// s := uint32(math.Floor((c.Timestamp / 1000) + 0.5))
// 向上取整 math.Ceil(x) 传入和返回值都是float64
// 向下取整 math.Floor(x) 传入和返回值都是float64
// s.TsExtInfo = math.Floor(float64(c.Timestamp-s.TsFirstTs) / 1000)
