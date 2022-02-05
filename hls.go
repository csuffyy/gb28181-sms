package main

import (
	"container/list"
	"fmt"
	"log"
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
	PmtPid             = 0x100
	VideoPid           = 0x101
	AudioPid           = 0x102
	VideoStreamId      = 0xe0
	AudioStreamId      = 0xc0
)

type HlsInfo struct {
	M3u8Path     string     // m3u8文件路径, 包含文件名
	M3u8File     *os.File   // m3u8文件描述符
	M3u8Data     string     // m3u8内容
	TsNum        uint32     // m3u8里ts的个数
	TsFirstSeq   uint32     // m3u8里第一个ts的序号
	TsLastSeq    uint32     // m3u8里最后一个ts的序号
	TsList       *list.List // 存储ts内容, 双向链表, 删头追尾
	TsFirstTs    uint32     // ts文件中第一个时间戳
	TsExtInfo    float64    // ts文件的播放时长
	TsPath       string     // ts文件路径, 包含文件名
	TsFile       *os.File   // ts文件描述符
	TsData       []byte     // ts文件内容(不完整，正在生成)
	VideoCounter uint8      // 4bit, 0x0 - 0xf 循环
	AudioCounter uint8      // 4bit, 0x0 - 0xf 循环
	SpsPpsData   []byte     // 视频关键帧tsPacket
	AdtsData     []byte     // 音频tsPacket需要
}

/**********************************************************/
/* rtmp流如何生成ts
/**********************************************************/
// 1 rtmp里的Metadata, ts不需要???
// 2 rtmp里的VideoHeader里有sps和pps, ts需要 放到关键帧前面, sps和pps前面要加 nalu标识0x00000001
// 3 rtmp里的视频数据是es(h264)裸流, ts要封装成pes, 数据前面要加 nalu标识0x00000001
// 4 rtmp里的AudioHeader里有音频流相关的信息, ts需要用它来生成adts
// 5 rtmp里的音频数据是es(aac)裸流，ts要封装成adts
// 6 tsFile是有很多个tsPakcet组成的，tsPacket的固定大小是188字节

/**********************************************************/
/* tsFile里 tsPacket的顺序和结构
/**********************************************************/
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
	s.log.Printf("%#v", AvcC)

	//sps内容里第一个就是0x67, pps内容里第一个就是0x68
	size := 8 + AvcC.SpsSize + AvcC.PpsSize
	s.SpsPpsData = make([]byte, size)
	Uint32ToByte(0x00000001, s.SpsPpsData[0:4], BE)
	copy(s.SpsPpsData[4:4+AvcC.SpsSize], AvcC.SpsData)
	sp := 4 + AvcC.SpsSize
	Uint32ToByte(0x00000001, s.SpsPpsData[sp:sp+4], BE)
	copy(s.SpsPpsData[sp+4:sp+4+AvcC.PpsSize], AvcC.PpsData)
	s.log.Printf("%x", s.SpsPpsData)
}

//ISO 14496-3, P122
//28bit固定头 + 28bit可变头 = 56bit, 7byte
// FF F9 50 80 2E 7F FC
// 11111111 11111001 01010000 10000000 00101110 01111111 11111100
// fff 1 00 1 01 0100 0 010 0 0 0 0 0000101110011 11111111111 00
// 366-2=364, 371-264=7, 7字节adts  0x173 = 371
type Adts struct {
	Syncword                     uint16 // 12bit, 固定值0xfff
	Id                           uint8  // 1bit, MPEG Version: 0 is MPEG-4, 1 is MPEG-2
	Layer                        uint8  // 2bit, 固定值00
	ProtectionAbsent             uint8  // 1bit, 0表示有CRC校验, 1表示没有CRC校验
	ProfileObjectType            uint8  // 2bit
	SamplingFrequencyIndex       uint8  // 4bit
	PrivateBit                   uint8  // 1bit
	ChannelConfiguration         uint8  // 3bit
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
	s.log.Printf("%#v", AacC)

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
	s.log.Printf("%#v", adts)

	s.AdtsData = make([]byte, 7)
	s.AdtsData[0] = 0xff
	s.AdtsData[1] = 0xf0 | (adts.Id&0x1)<<3 | (adts.Layer&0x3)<<1 | (adts.ProtectionAbsent & 0x1)
	s.AdtsData[2] = (adts.ProfileObjectType&0x3)<<6 | (adts.SamplingFrequencyIndex&0xf)<<2 | (adts.PrivateBit&0x1)<<1 | (adts.ChannelConfiguration&0x04)>>2
	s.AdtsData[3] = (adts.ChannelConfiguration&0x03)<<6 | (adts.OriginalCopy&0x1)<<5 | (adts.Home&0x1)<<4 | (adts.CopyrightIdentificationBit&0x1)<<3 | (adts.CopyrightIdentificationStart&0x1)<<2 | uint8((adts.AacFrameLength>>11)&0x3)
	s.AdtsData[4] = uint8((adts.AacFrameLength >> 3) & 0xff)
	s.AdtsData[5] = uint8(adts.AacFrameLength&0x7)<<5 | uint8((adts.AdtsBufferFullness>>6)&0x1f)
	s.AdtsData[6] = uint8((adts.AdtsBufferFullness&0x3f)<<2) | (adts.NumberOfRawDataBlocksInFrame & 0x3)
	s.log.Printf("%x", s.AdtsData)
}

// size = 7字节adts头 + aac数据长度
func SetAdtsLength(d []byte, size uint16) {
	d[3] = (d[3] & 0xfc) | uint8((size>>11)&0x2) // 最右2bit
	d[4] = (d[4] & 0x00) | uint8((size>>3)&0xff) // 8bit
	d[5] = (d[5] & 0x1f) | uint8((size&0x7)<<5)  // 最左3bit
}

/**********************************************************/
/* HlsCreator()
/**********************************************************/
func HlsCreator(s *Stream) {
	var i uint32 = 0
	for {
		c, ok := <-s.HlsChan
		if !ok {
			s.log.Println("HlsCreator close")
			return
		}
		s.log.Printf("-------------------->> chunk %d", i)
		i++
		s.log.Printf("===>> fmt=%d, csid=%d, timestamp=%d, MsgLength=%d, MsgTypeId=%d, DataType=%s", c.Fmt, c.Csid, c.Timestamp, c.MsgLength, c.MsgTypeId, c.DataType)

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
//0x0001  表示CAT
//0x1fff	表示空包
//===> AdaptationFieldControl
//00：是保留值
//01：负载中只有有效载荷
//10：负载中只有自适应字段
//11：先有自适应字段，再有有效载荷
// 8 + 3 + 13 + 2 + 2 + 4 = 4byte
type TsHeader struct {
	SyncByte                   uint8  // 8bit, 同步字节 固定为0x47
	TransportErrorIndicator    uint8  // 1bit, 传输错误标志
	PayloadUnitStartIndicator  uint8  // 1bit, 负载单元开始标志
	TransportPriority          uint8  // 1bit, 传输优先级
	PID                        uint16 // 13bit, TS包负载的数据类型
	TransportScramblingControl uint8  // 2bit, 传输加扰控制
	AdaptationFieldControl     uint8  // 2bit, 适应域控制
	ContinuityCounter          uint8  // 4bit, 连续计数器
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

// 33 + 6 + 9 = 48bit, 6Byte
type Pcr struct {
	ProgramClockReferenceBase      uint64 // 33bit
	Reserved                       uint8  // 6bit
	ProgramClockReferenceExtension uint16 // 9bit
}

//---------------------------------------------------------/
// tsPakcet
//---------------------------------------------------------/
// tsPacket 大小固定188byte, tsHeader 固定4byte
// TsPacketCreate() 只有tsHeader + 184字节数据
// 返回tsData 和 写入tsData的字节数(也就是data数据消耗了多少)
func TsPacketCreate(s *Stream, pid uint16, data []byte) ([]byte, int) {
	var th TsHeader
	th.SyncByte = 0x47
	th.TransportErrorIndicator = 0x0
	th.PayloadUnitStartIndicator = 0x0
	th.TransportPriority = 0x0
	th.PID = pid
	th.TransportScramblingControl = 0x0
	th.AdaptationFieldControl = 0x1
	if pid == AudioPid {
		th.ContinuityCounter = s.AudioCounter
		s.AudioCounter++
		if s.AudioCounter > 0xf {
			s.AudioCounter = 0x0
		}
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

	dataLen := len(data)
	freeBuffLen := 188 - 4
	if dataLen >= freeBuffLen {
		dataLen = freeBuffLen
		copy(tsData[4:4+dataLen], data)
		freeBuffLen = 0
	} else {
		copy(tsData[4:4+dataLen], data)
		freeBuffLen = 188 - 4 - dataLen
		for i := 0; i < freeBuffLen; i++ {
			tsData[i+4+dataLen] = 0xff
		}
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
		freeBuffLen = 0
	} else {
		copy(tsData[5:5+dataLen], data)
		freeBuffLen = 188 - 5 - dataLen
		for i := 0; i < freeBuffLen; i++ {
			tsData[5+dataLen+i] = 0xff
		}
	}

	//s.log.Printf("%x", tsData)
	s.log.Printf("TsHeaderLen=4, SeparatorLen=1, DataLen=%d, PaddingLen=%d", dataLen, freeBuffLen)
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
		freeBuffLen = 0
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
		freeBuffLen = 0
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
	var th TsHeader
	th.SyncByte = 0x47
	th.TransportErrorIndicator = 0x0
	th.PayloadUnitStartIndicator = 0x1
	th.TransportPriority = 0x0
	th.PID = pid
	th.TransportScramblingControl = 0x0
	th.AdaptationFieldControl = 0x1
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

	dataLen := len(data)
	freeBuffLen := 188 - 4
	if dataLen >= freeBuffLen {
		dataLen = freeBuffLen
		copy(tsData[4:4+dataLen], data)
		freeBuffLen = 0
	} else {
		copy(tsData[4:4+dataLen], data)
		freeBuffLen = 188 - 4 - dataLen
		for i := 0; i < freeBuffLen; i++ {
			tsData[i+4+dataLen] = 0xff
		}
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

	s.TsPath = fmt.Sprintf("%s/hls/%s_%d.ts", s.Key, s.Key, s.TsLastSeq)
	s.log.Println(s.TsPath)

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
		s.log.Printf("Write ts fail, %s", err)
		return
	}

	_, pmtData := PmtCreate()
	s.TsData, _ = TsPacketCreatePatPmt(s, PmtPid, pmtData)
	_, err = s.TsFile.Write(s.TsData)
	if err != nil {
		s.log.Printf("Write ts fail, %s", err)
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
		s.log.Printf("Write ts fail, %s", err)
		return
	}

	for {
		s.TsData, consumeLen = TsPacketCreate(s, VideoPid, pesData[start:])
		start += consumeLen

		_, err := s.TsFile.Write(s.TsData)
		if err != nil {
			s.log.Printf("Write ts fail, %s", err)
			return
		}

		if start >= pesDataLen {
			s.log.Printf("pesDataLen=%d, start=%d", pesDataLen, start)
			break
		}
	}
}

func TsFileAppendInterFrame(s *Stream, c *Chunk) {
	_, pesHeaderData := PesHeaderCreate(s, c)
	pesData := PesDataCreateInterFrame(s, c, pesHeaderData)

	var pesDataLen = len(pesData)
	//SetPesPakcetLength(pesData, uint16(pesDataLen)-6)

	var consumeLen, start int
	s.TsData, consumeLen = TsPacketCreateInterFrame(s, VideoPid, pesData[start:])
	start += consumeLen

	_, err := s.TsFile.Write(s.TsData)
	if err != nil {
		s.log.Printf("Write ts fail, %s", err)
		return
	}

	for {
		s.TsData, consumeLen = TsPacketCreate(s, VideoPid, pesData[start:])
		start += consumeLen

		_, err := s.TsFile.Write(s.TsData)
		if err != nil {
			s.log.Printf("Write ts fail, %s", err)
			return
		}

		if start >= pesDataLen {
			s.log.Printf("pesDataLen=%d, start=%d", pesDataLen, start)
			break
		}
	}
}

func TsFileAppendAacFrame(s *Stream, c *Chunk) {
	_, pesHeaderData := PesHeaderCreate(s, c)
	pesData := PesDataCreateAacFrame(s, c, pesHeaderData)

	var pesDataLen = len(pesData)
	//SetPesPakcetLength(pesData, uint16(pesDataLen)-6)

	var consumeLen, start int
	s.TsData, consumeLen = TsPacketCreateAacFrame(s, AudioPid, pesData[start:])
	start += consumeLen

	_, err := s.TsFile.Write(s.TsData)
	if err != nil {
		s.log.Printf("Write ts fail, %s", err)
		return
	}

	for {
		s.TsData, consumeLen = TsPacketCreate(s, AudioPid, pesData[start:])
		start += consumeLen

		_, err := s.TsFile.Write(s.TsData)
		if err != nil {
			s.log.Printf("Write ts fail, %s", err)
			return
		}

		if start >= pesDataLen {
			s.log.Printf("pesDataLen=%d, start=%d", pesDataLen, start)
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
	s.log.Printf("c.Timestamp=%d, s.TsFirstTs=%d, s.TsExtInfo=%f, conf.HlsTsMaxTime=%d", c.Timestamp, s.TsFirstTs, s.TsExtInfo, conf.HlsTsMaxTime)

	var tf bool
	if s.TsPath == "" || uint32(s.TsExtInfo) >= conf.HlsTsMaxTime {
		s.log.Println("--->> TsFileCreate()")
		TsFileCreate(s, c) // 新建TsFile, 并写入
		tf = true
	} else {
		s.log.Println("--->> TsFileAppend()")
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

type OptionalPesHeader struct {
	FixedValue0            uint8 // 2bit, 固定值0x2
	PesScramblingControl   uint8 // 2bit, 加扰控制
	PesPriority            uint8 // 1bit, 优先级
	DataAlignmentIndicator uint8 // 1bit,
	Copyright              uint8 // 1bit
	OriginalOrCopy         uint8 // 1bit, 原始或复制
	PtsDtsFlags            uint8 // 2bit, 时间戳标志位, 00表示没有对应的信息; 01是被禁用的; 10表示只有PTS; 11表示有PTS和DTS
	EscrFlag               uint8 // 1bit
	EsRateFlag             uint8 // 1bit
	DsmTrickModeFlag       uint8 // 1bit
	AdditionalCopyInfoFlag uint8 // 1bit
	PesCrcFlag             uint8 // 1bit
	PesExtensionFlag       uint8 // 1bit
	PesHeaderDataLength    uint8 // 8bit, 表示后面还有x个字节, 之后就是负载数据
}

// PtsDtsFlags            uint8  // 2bit
// 0x0 00, 没有PTS和DTS
// 0x1 01, 禁止使用
// 0x2 10, 只有PTS
// 0x3 11, 有PTS 有DTS
// 6 + 3 + 5 = 14byte
// 6 + 3 + 5 + 5 = 19byte
type PesHeader struct {
	PacketStartCodePrefix uint32 // 24bit, 固定值 0x000001
	StreamId              uint8  // 8bit
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
	s.log.Printf("c.DataType=%s, pts=%d, dts=%d, CompositionTime=%d", c.DataType, pts, dts, CompositionTime)

	var pes PesHeader
	pes.PacketStartCodePrefix = 0x000001
	if c.MsgTypeId == MsgTypeIdAudio { // 8
		pes.StreamId = AudioStreamId
	}
	if c.MsgTypeId == MsgTypeIdVideo { // 9
		pes.StreamId = VideoStreamId
	}
	pes.PtsDtsFlags = 0x2 // 只有PTS, 40bit
	pes.PesHeaderDataLength = 5
	pes.PesPacketLength = 3 + 5
	if pts != dts {
		pes.PtsDtsFlags = 0x3 // 有PTS 有DTS, 40bit + 40bit
		pes.PesHeaderDataLength = 10
		pes.PesPacketLength = 3 + 10
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
	s.log.Printf("%#v", pes)

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

	if pes.PtsDtsFlags == 0x2 { // 只有PTS, 40bit
		pes.FixedValue1 = 0x10
		pesData[9] = (pes.FixedValue1&0xf)<<4 | uint8((pes.Pts>>29)&0xd) | (pes.MarkerBit0 & 0x1)
		pesData[10] = uint8((pes.Pts >> 22) & 0xff)
		pesData[11] = uint8((pes.Pts>>14)&0xfe) | (pes.MarkerBit1 & 0x1)
		pesData[12] = uint8((pes.Pts >> 7) & 0xff)
		pesData[13] = uint8((pes.Pts&0x7F)<<1) | (pes.MarkerBit2 & 0x1)
	}
	if pes.PtsDtsFlags == 0x3 { // 有PTS 有DTS, 40bit + 40bit
		pes.FixedValue1 = 0x11
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
	dataLen := pesHeaderDataLen + SpsPpsDataLen + 6 + 4 + MsgDataLen
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
	ee += 4
	Uint32ToByte(0x00000001, data[ss:ee], BE)
	ss = ee
	ee += MsgDataLen
	copy(data[ss:], c.MsgData[9:])
	return data
}

func PesDataCreateInterFrame(s *Stream, c *Chunk, phd []byte) []byte {
	pesHeaderDataLen := len(phd)
	MsgDataLen := int(c.MsgLength) - 9
	dataLen := pesHeaderDataLen + 6 + 4 + MsgDataLen
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
	ee += 4
	Uint32ToByte(0x00000001, data[ss:ee], BE)
	ss = ee
	ee += MsgDataLen
	copy(data[ss:], c.MsgData[9:])
	return data
}

func PesDataCreateAacFrame(s *Stream, c *Chunk, phd []byte) []byte {
	pesHeaderDataLen := len(phd)
	MsgDataLen := int(c.MsgLength) - 2
	s.log.Printf("MsgDataLen=%d", MsgDataLen)
	dataLen := pesHeaderDataLen + 7 + MsgDataLen
	data := make([]byte, dataLen)

	ss := 0
	ee := pesHeaderDataLen
	copy(data[ss:ee], phd)
	ss = ee
	ee += 7
	//SetAdtsLength(s.AdtsData, uint16(7+MsgDataLen))
	size := uint16(7 + MsgDataLen)
	s.AdtsData[3] = (s.AdtsData[3] & 0xfc) | uint8((size>>11)&0x3) // 最右2bit
	s.AdtsData[4] = (s.AdtsData[4] & 0x00) | uint8((size>>3)&0xff) // 8bit
	s.AdtsData[5] = (s.AdtsData[5] & 0x1f) | uint8((size&0x7)<<5)  // 最左3bit
	copy(data[ss:ee], s.AdtsData)
	ss = ee
	ee += MsgDataLen
	copy(data[ss:], c.MsgData[2:])
	//s.log.Printf("%x", data)
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
	TableId                uint8  // 8bit, 3byte
	SectionSyntaxIndicator uint8  // 1bit
	Zero                   uint8  // 1bit
	Reserved0              uint8  // 2bit
	SectionLength          uint16 // 12bit, 5byte
	TransportStreamId      uint16 // 16bit
	Reserved1              uint8  // 2bit
	VersionNumber          uint8  // 5bit
	CurrentNextIndicator   uint8  // 1bit
	SectionNumber          uint8  // 8bit
	LastSectionNumber      uint8  // 8bit
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
	pat.ProgramNumber = 1
	pat.Reserved2 = 0x3
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
type PmtStream struct {
	StreamType    uint8  // 8bit, arr 5byte
	Reserved4     uint8  // 3bit, arr
	ElementaryPID uint16 // 13bit, arr
	Reserved5     uint8  // 4bit, arr
	EsInfoLength  uint16 // 12bit, arr
}

// 3 + 9 + 5*2 + 4 = 26byte
type Pmt struct {
	TableId                uint8  // 8bit, 3byte
	SectionSyntaxIndicator uint8  // 1bit
	Zero                   uint8  // 1bit
	Reserved0              uint8  // 2bit
	SectionLength          uint16 // 12bit, 9byte
	ProgramNumber          uint16 // 16bit
	Reserved1              uint8  // 2bit
	VersionNumber          uint8  // 5bit
	CurrentNextIndicator   uint8  // 1bit
	SectionNumber          uint8  // 8bit
	LastSectionNumber      uint8  // 8bit
	Reserved2              uint8  // 3bit
	PcrPID                 uint16 // 13bit
	Reserved3              uint8  // 4bit
	ProgramInfoLength      uint16 // 12bit
	PmtStream              []PmtStream
	CRC32                  uint32 // 32bit
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
	pmt.PcrPID = 0x101
	pmt.Reserved3 = 0xf
	pmt.ProgramInfoLength = 0x0
	pmt.PmtStream = make([]PmtStream, 2)
	pmt.PmtStream[0].StreamType = 0x1b // AVC video stream as defined in ITU-T Rec. H.264 | ISO/IEC 14496-10 Video
	pmt.PmtStream[0].Reserved4 = 0x7
	pmt.PmtStream[0].ElementaryPID = VideoPid
	pmt.PmtStream[0].Reserved5 = 0xf
	pmt.PmtStream[0].EsInfoLength = 0x0
	pmt.PmtStream[1].StreamType = 0xf // ISO/IEC 13818-7 Audio with ADTS transport syntax
	pmt.PmtStream[1].Reserved4 = 0x7
	pmt.PmtStream[1].ElementaryPID = AudioPid
	pmt.PmtStream[1].Reserved5 = 0xf
	pmt.PmtStream[1].EsInfoLength = 0x0
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

	s.M3u8Data = fmt.Sprintf(m3u8Head, conf.HlsTsMaxTime, s.TsFirstSeq)

	var tis string
	for e := s.TsList.Front(); e != nil; e = e.Next() {
		ti = (e.Value).(TsInfo)
		tis = fmt.Sprintf("%s\n%s", tis, ti.TsInfoStr)
	}
	s.M3u8Data = fmt.Sprintf("%s%s", s.M3u8Data, tis)
	//s.log.Println(s.M3u8Data)

	// 清空文件
	err := s.M3u8File.Truncate(0)
	if err != nil {
		s.log.Println(err)
		return
	}
	_, err = s.M3u8File.Seek(0, 0)
	if err != nil {
		s.log.Println(err)
		return
	}
	// 写入文件
	_, err = s.M3u8File.WriteString(s.M3u8Data)
	if err != nil {
		s.log.Printf("Write %s fail, %s", s.M3u8Path, err)
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
	app, stream, fn := GetPlayInfo(r.URL.String())
	file := fmt.Sprintf("%s_%s/hls/%s_%s.m3u8", app, stream, app, stream)
	log.Println(app, stream, fn, file)

	d, err := utils.ReadAllFile(file)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return d, nil
}

func GetTs(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	app, stream, fn := GetPlayInfo(r.URL.String())
	file := fmt.Sprintf("%s_%s/hls/%s", app, stream, fn)
	log.Println(app, stream, fn, file)

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
// 向下取整
// math.Floor(x) 向下取整，返回值是float32
// s.TsExtInfo = math.Floor(float64(c.Timestamp-s.TsFirstTs) / 1000)
