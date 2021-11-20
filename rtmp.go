package main

import (
	"bytes"
	"container/list"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net"
)

const (
	MsgTypeIdSetChunkSize     = 1  //默认128byte, 最大16777215(0xFFFFFF)
	MsgTypeIdAbort            = 2  //终止消息
	MsgTypeIdAck              = 3  //回执消息
	MsgTypeIdUserControl      = 4  //用户控制消息
	MsgTypeIdWindowAckSize    = 5  //窗口大小
	MsgTypeIdSetPeerBandwidth = 6  //设置对端带宽
	MsgTypeIdAudio            = 8  //音频消息
	MsgTypeIdVideo            = 9  //视频消息
	MsgTypeIdDataAmf3         = 15 //AMF3数据消息
	MsgTypeIdDataAmf0         = 18 //AMF0数据消息
	MsgTypeIdShareAmf3        = 16 //AMF3共享对象消息
	MsgTypeIdShareAmf0        = 19 //AMF0共享对象消息
	MsgTypeIdCmdAmf3          = 17 //AMF3命令消息
	MsgTypeIdCmdAmf0          = 20 //AMF0命令消息
)

var (
	FPKey = []byte{
		// Genuine Adobe Flash Player 001
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd',
		'o', 'b', 'e', ' ', 'F', 'l', 'a', 's', 'h', ' ',
		'P', 'l', 'a', 'y', 'e', 'r', ' ', '0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00,
		0xD0, 0xD1, 0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D,
		0x29, 0x80, 0x6F, 0xAB, 0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB,
		0x31, 0xAE,
	}
	FMSKey = []byte{
		// Genuine Adobe Flash Media Server 001
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd',
		'o', 'b', 'e', ' ', 'F', 'l', 'a', 's', 'h', ' ',
		'M', 'e', 'd', 'i', 'a', ' ', 'S', 'e', 'r', 'v',
		'e', 'r', ' ', '0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00,
		0xD0, 0xD1, 0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D,
		0x29, 0x80, 0x6F, 0xAB, 0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB,
		0x31, 0xAE,
	}
	FPKeyP  = FPKey[:30]
	FMSKeyP = FMSKey[:36]

	//RtmpPlayerKey = []byte{
	//RtmpServerKey = []byte{
	//RtmpPlayerPartKey = RtmpPlayerKey[:30]
	//RtmpServerPartKey = RtmpServerKey[:36]
)

type Stream struct {
	Conn                net.Conn
	ChunkSize           uint32
	WindowAckSize       uint32
	RemoteChunkSize     uint32
	RemoteWindowAckSize uint32
	Chunks              map[uint32]Chunk
	AmfInfo             AmfInfo
	IsPublisher         bool // true为发布者，false为播放者
	MessageHandleDone   bool
	RecvMsgLen          uint32 // 用于ACK回应,接收消息的总长度(不包括ChunkHeader)
	TransmitSwitch      string
	Players             map[string]*Stream // key use player's ip_port
	NewPlayer           bool               // player use, 新来的播放者要先发GopCache
	GopCache
}

// 要缓存的数据
// 1 Metadata
// 2 Video Header
// 3 Audio Header
// 4 MediaData 里面有 I/B/P帧和音频帧, 按来的顺序存放，
//   MediaData 里内容举例：I B P B A A B P I ...
//MediaData里 最多有 GopCacheMax + 1 个 Gop的数据
//比如GopCacheMax=2, 那么MediaData里最多有3个Gop, 第3个Gop不完整, 这样做发送时方便
//当第4个Gop的关键帧到达的时，删除第1个Gop的数据
type GopCache struct {
	GopCacheMax int // 最多缓存几个Gop, 默认为1个
	GopCacheNum int // VideoData里 I帧的个数
	MetaData    *Chunk
	VideoHeader *Chunk
	AudioHeader *Chunk
	MediaData   *list.List // 双向链表, FIXME: 写的时候不能读
}

type Chunk struct {
	Fmt         uint32 // format
	Csid        uint32 // chunk stream id
	Timestamp   uint32
	TimeExted   bool
	TimeDelta   uint32
	MsgLength   uint32
	MsgTypeId   uint32
	MsgStreamId uint32 // 小端字节序
	MsgData     []byte
	MsgIndex    uint32 // 接收的数据 写到MsgData的哪里
	MsgRemain   uint32 // MsgData 还有多少数据需要接收
	Full        bool
	DataType    string // "VideoKeyFrame", "VideoInterFrame", "AudioAacFrame"
	//IsKeyFrame  bool   // GopCache删除过期数据时使用
}

func RtmpServer() {
	log.Println("start rtmp listen on", conf.RtmpListen)
	l, err := net.Listen("tcp", conf.RtmpListen)
	if err != nil {
		log.Fatalln(err)
	}

	for {
		c, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println("------>>> new rtmp connect")
		log.Println(c.RemoteAddr().String())
		//c.Close()

		go RtmpHandler(c)
	}
}

func RtmpHandler(c net.Conn) {
	// 这里还无法区分是 rtmp推流 或 rtmp播放
	s := NewStream(c)

	if err := RtmpHandshakeServer(s); err != nil {
		log.Println(err)
		return
	}
	log.Println("RtmpHandshakeServer ok")

	if err := RtmpHandleMessage(s); err != nil {
		log.Println(err)
		return
	}
	log.Println("RtmpHandleMessage ok")

	//log.Printf("%#v", s)
	log.Printf("the stream have %d chunks", len(s.Chunks))
	log.Printf("the stream is publisher %t", s.IsPublisher)
	if s.IsPublisher {
		// for循环负责接收数据，发送如何处理 ???
		// 方法1: 发送也在 for循环里做，这样可能会影响接收
		// 方法2: 每个播放一个发送协程, 这样发送协程可能会太多
		// 方法3: 所有播放共用一个发送协程, 进入for循环前 创建一个发送协程
		RtmpPublisher(s) // 进入for循环 接收数据并拷贝给发送协程
	} else {
		RtmpPlayer(s) // 只需把播放信息 存入到Publisher的map里
	}
}

func NewStream(c net.Conn) *Stream {
	s := &Stream{
		Conn:                c,
		ChunkSize:           128,
		WindowAckSize:       2500000,
		RemoteChunkSize:     128,
		RemoteWindowAckSize: 2500000,
		Chunks:              make(map[uint32]Chunk),
		Players:             make(map[string]*Stream),
		GopCache:            GopCacheNew(),
	}
	return s
}

/////////////////////////////////////////////////////////////////
// rtmp publisher
/////////////////////////////////////////////////////////////////
func RtmpPublisher(s *Stream) {
	key := fmt.Sprintf("%s_%s", s.AmfInfo.App, s.AmfInfo.PublishName)
	log.Println(key)

	_, ok := Publishers[key]
	if ok { // 发布者已存在, 断开连接并返回错误
		log.Printf("publisher %s is exist", key)
		s.Conn.Close()
		return
	}
	Publishers[key] = s

	//go RtmpSender(s) // 给所有播放者发送数据

	s.TransmitSwitch = "on"
	i := 0
	for {
		log.Println("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx>>>", i)
		if s.TransmitSwitch == "off" {
			log.Printf("publisher %s off", key)
			s.Conn.Close()
			return
		}

		// 接收数据
		if err := RtmpReceiver(s); err != nil {
			log.Println(err)
			s.Conn.Close()
			return
		}

		// 0 is MetaData;
		// 1 is video AVC sequence header
		// 2 is audio AAC sequence header;
		// 3 VideoKeyFrame 24665
		// 4 VideoInterFrame 5137
		// 5 AudioAacFrame 366
		// 6 AudioAacFrame 367
		// 7 VideoInterFrame 714
		// 8 AudioAacFrame 366
		if i == 203 {
			//s.Conn.Close()
			//break
		}
		i++
		//s.DataChan <- p
	}
}

func RtmpReceiver(s *Stream) error {
	var i uint32
	var c Chunk
	var err error
	for {
		log.Println("==============================>>>", i)
		i++

		// 接收 并 合并 数据
		if c, err = MessageMerge(s, nil); err != nil {
			log.Println(err)
			return err
		}

		SendAckMessage(s, c.MsgLength)

		log.Printf("Message TypeId %d, len %d", c.MsgTypeId, c.MsgLength)
		if c.MsgTypeId == MsgTypeIdAudio || // 8
			c.MsgTypeId == MsgTypeIdVideo || // 9
			c.MsgTypeId == MsgTypeIdDataAmf3 || // 15
			c.MsgTypeId == MsgTypeIdDataAmf0 { // 18
			break
		}
	}

	// 处理音频数据 audio
	if c.MsgTypeId == MsgTypeIdAudio { // 8
		AudioHandle(s, &c)
	}
	// 处理视频数据 video
	if c.MsgTypeId == MsgTypeIdVideo { // 9
		VideoHandle(s, &c)
	}
	// 处理元数据 Metadata
	if c.MsgTypeId == MsgTypeIdDataAmf3 || // 15
		c.MsgTypeId == MsgTypeIdDataAmf0 { // 18
		MetadataHandle(s, &c)
	}

	log.Println(s.GopCacheMax, s.GopCacheNum, s.MediaData.Len())
	PrintList(s.MediaData)
	return nil
}

func PrintList(l *list.List) {
	i := 0
	for e := l.Front(); e != nil; e = e.Next() {
		v := (e.Value).(*Chunk)
		log.Println(i, v.DataType, v.MsgLength)
		i++
	}
}

// >>> SoundFormat <<<
//0 = Linear PCM, platform endian
//1 = ADPCM
//2 = MP3
//3 = Linear PCM, little endian
//4 = Nellymoser 16-kHz mono
//5 = Nellymoser 8-kHz mono
//6 = Nellymoser
//7 = G.711 A-law logarithmic PCM 8 = G.711 mu-law logarithmic PCM 9 = reserved
//10 = AAC
//11 = Speex
//14 = MP3 8-Khz
//15 = Device-specific sound
// >>> SoundRate <<<
//0 = 5.5-kHz
//1 = 11-kHz
//2 = 22-kHz
//3 = 44-kHz For AAC: always 3
// >>> SoundSize <<<
//0 = snd8Bit
//1 = snd16Bit
// >>> SoundType <<<
//0 = sndMono, 单声道
//1 = sndStereo, 双声道(立体声)
func AudioHandle(s *Stream, c *Chunk) error {
	SoundFormat := (c.MsgData[0] & 0xF0) >> 4 // 4bit
	SoundRate := (c.MsgData[0] & 0xC) >> 2    // 2bit
	SoundSize := (c.MsgData[0] & 0x2) >> 1    // 1bit
	SoundType := c.MsgData[0] & 0x1           // 1bit

	if SoundFormat == 10 {
		log.Println("SoundFormat is AAC")
	} else {
		err := fmt.Errorf("untreated SoundFormat %d", SoundFormat)
		log.Println(err)
		return err
	}

	//0: AAC sequence header
	//1: AAC raw
	AACPacketType := c.MsgData[1]
	// 10, 3, 1, 1, 0/1
	log.Println(SoundFormat, SoundRate, SoundSize, SoundType, AACPacketType)

	if AACPacketType == 0 {
		log.Println("This frame is AAC sequence header")

		// AudioSpecificConfig is explained in ISO 14496-3
		var AacC AudioSpecificConfig
		AacC.ObjectType = (c.MsgData[2] & 0xF8) >> 3 // 5bit
		AacC.SamplingIdx =
			((c.MsgData[2] & 0x7) << 1) | (c.MsgData[3] >> 7) // 4bit
		AacC.ChannelNum = (c.MsgData[3] & 0x78) >> 3     // 4bit
		AacC.FrameLenFlag = (c.MsgData[3] & 0x4) >> 2    // 1bit
		AacC.DependCoreCoder = (c.MsgData[3] & 0x2) >> 1 // 1bit
		AacC.ExtensionFlag = c.MsgData[3] & 0x1          // 1bit
		// 2, 4, 2, 0(1024), 0, 0
		log.Printf("%#v", AacC)

		s.GopCache.AudioHeader = c
	} else {
		// Raw AAC frame data
		log.Println("This frame is AAC raw")
		c.DataType = "AudioAacFrame"
		s.GopCache.MediaData.PushBack(c)
	}
	return nil
}

type AudioSpecificConfig struct {
	ObjectType      uint8 // 5bit
	SamplingIdx     uint8 // 4bit
	ChannelNum      uint8 // 4bit
	FrameLenFlag    uint8 // 1bit
	DependCoreCoder uint8 // 1bit
	ExtensionFlag   uint8 // 1bit
}

func GopCacheNew() GopCache {
	return GopCache{
		GopCacheMax: 1,
		MediaData:   list.New(),
	}
}

func GopCacheUpdate(s *Stream) {
	// 1 先判断CacheData里的关键帧个数 是否达到GopCacheMax+1, 如果没有就直接存入并退出
	// 2 如果达到, 就先删除CacheData里最早的Gop(含音频帧), 然后再存入
	gc := s.GopCache
	if gc.GopCacheNum < gc.GopCacheMax+1 {
		return
	}

	KeyFrameNum := 0
	for e := gc.MediaData.Front(); e != nil; e = e.Next() {
		v := (e.Value).(*Chunk)
		if v.DataType == "VideoKeyFrame" {
			KeyFrameNum++
		}
		if KeyFrameNum == 2 {
			break
		}
		gc.MediaData.Remove(e)
	}
	s.GopCache.GopCacheNum--
}

func GopCacheSend() {
	// 1 发送Metadata
	// 2 发送VideoHeader
	// 3 发送AudioHeader
	// 4 发送MediaData(包含最后收到的数据)
}

func VideoHandle(s *Stream, c *Chunk) error {
	FrameType := c.MsgData[0] >> 4 // 4bit
	CodecId := c.MsgData[0] & 0xf  // 4bit
	log.Printf("FrameType=%d, CodecId=%d", FrameType, CodecId)

	//1: keyframe (for AVC, a seekable frame), 关键帧(I帧)
	//2: inter frame (for AVC, a non- seekable frame), 非关键帧(P/B帧)
	//3: disposable inter frame (H.263 only)
	//4: generated keyframe (reserved for server use only)
	//5: video info/command frame
	if FrameType == 1 {
		log.Println("FrameType is KeyFrame(I frame)")
		c.DataType = "VideoKeyFrame"
	} else if FrameType == 2 {
		// TODO: 如何区分是 B帧 还是 P帧
		log.Println("FrameType is InterFrame(B/P frame)")
		c.DataType = "VideoInterFrame"
	} else {
		err := fmt.Errorf("untreated FrameType %d", FrameType)
		log.Println(err)
		return err
	}

	//1: JPEG (currently unused)
	//2: Sorenson H.263
	//3: Screen video
	//4: On2 VP6
	//5: On2 VP6 with alpha channel
	//6: Screen video version 2
	//7: AVC, AVCVIDEOPACKET
	if CodecId != 7 {
		err := fmt.Errorf("CodecId is't AVC")
		log.Println(err)
		return err
	}

	//0: AVC sequence header
	//1: AVC NALU
	//2: AVC end of sequence
	AVCPacketType := c.MsgData[1] // 8bit
	//创作时间 int24
	CompositionTime := ByteToUint32(c.MsgData[2:5], BE) // 24bit

	if AVCPacketType == 0 {
		log.Println("This frame is AVC sequence header")

		// 前5个字节上面已经处理，AVC sequence header从第6个字节开始
		//0x17, 0x00, 0x00, 0x00, 0x00, 0x01, 0x4d, 0x40, 0x1f, 0xff,
		//0xe1, 0x00, 0x1c, 0x67, 0x4d, 0x40, 0x1f, 0xe8, 0x80, 0x28,
		//0x02, 0xdd, 0x80, 0xb5, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00,
		//0x03, 0x00, 0x40, 0x00, 0x00, 0x0c, 0x03, 0xc6, 0x0c, 0x44,
		//0x80, 0x01, 0x00, 0x04, 0x68, 0xeb, 0xef, 0x20
		//See ISO 14496-15, 5.2.4.1 for AVCDecoderConfigurationRecord
		//ISO/IEC 14496-15:2019 要花钱购买
		//https://www.iso.org/standard/74429.html
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
		log.Printf("%#v", AvcC)

		s.GopCache.VideoHeader = c
	} else if AVCPacketType == 1 {
		// One or more NALUs
		log.Println("This frame is AVC NALU")

		//KeyFrameNum	1	2	3	4	关键帧个数
		//CacheNum		0	1	2	3	Gop个数 为2的时候要开始删Gop了
		//CacheMax		1	1	1	1	Gop最大个数
		if FrameType == 1 {
			if s.GopCache.MediaData.Len() > 0 {
				s.GopCache.GopCacheNum++
			}
			// 这里触发更新 CacheData
			GopCacheUpdate(s)
		}
		s.GopCache.MediaData.PushBack(c)
	} else {
		// Empty
		log.Println("This frame is AVC end of sequence")
	}

	log.Printf("AVCPacketType=%d, Composition=%d, DataLen=%d",
		AVCPacketType, CompositionTime, len(c.MsgData[5:]))
	return nil
}

type AVCDecoderConfigurationRecord struct {
	ConfigurationVersion uint8  // 8bit, 0x01
	AVCProfileIndication uint8  // 8bit, 0x4d, 0100 1101
	ProfileCompatibility uint8  // 8bit, 0x40, 0100 0000
	AVCLevelIndication   uint8  // 8bit, 0x1f
	Reserved0            uint8  // 6bit, 0xff, 1111 1111
	LengthSizeMinuxOne   uint8  // 2bit, 0xff
	Reserved1            uint8  // 3bit, 0xe1, 11100001
	NumOfSps             uint8  // 5bit, 0xe1
	SpsSize              uint16 // 16bit, 0x001c
	SpsData              []byte // 28Byte
	NumOsPps             uint8  // 8bit, 0x01
	PpsSize              uint16 // 16bit, 0x0004
	PpsData              []byte // 4Byte
}

// Metadata 数据要缓存起来，发送给播放者
func MetadataHandle(s *Stream, c *Chunk) error {
	r := bytes.NewReader(c.MsgData)
	vs, err := AmfUnmarshal(r) // 序列化转结构化
	if err != nil && err != io.EOF {
		log.Println(err)
		return err
	}
	log.Printf("Amf Unmarshal %#v", vs)

	s.GopCache.MetaData = c
	return nil
}

// 启播方式： 默认采用快速启播
// 1 快速启播：先发送缓存的gop数据, 再发送最新数据. 启播快 但延时交高
// 2 低延时启播：直接发送最新数据. 启播交慢 但是延时最低
func RtmpSender(s *Stream) {
	log.Println(len(s.Players))
	for _, p := range s.Players {
		// 新播放者，先发送缓存的gop数据，再发送最新数据
		if p.NewPlayer == true {
			p.NewPlayer = false
			//RtmpSendGOP()
		}
		// 老播放者，直接发送最新数据
		// send new packet
	}
}

/////////////////////////////////////////////////////////////////
// rtmp player
/////////////////////////////////////////////////////////////////
func RtmpPlayer(s *Stream) {
	key := fmt.Sprintf("%s_%s", s.AmfInfo.App, s.AmfInfo.StreamName)
	log.Println(key)

	p, ok := Publishers[key]
	if !ok { // 发布者不存在, 断开连接并返回错误
		log.Printf("publisher %s isn't exist", key)
		s.Conn.Close()
		return
	}

	key = fmt.Sprintf("%s_%s_%s", s.AmfInfo.App, s.AmfInfo.StreamName,
		s.Conn.RemoteAddr().String())
	log.Println(key)
	p.Players[key] = s
}

/////////////////////////////////////////////////////////////////
// rtmp handshake
/////////////////////////////////////////////////////////////////
func RtmpHandshakeServer(s *Stream) (err error) {
	var C0C1C2S0S1S2 [(1 + 1536*2) * 2]byte

	C0C1C2 := C0C1C2S0S1S2[:1536*2+1]
	C0 := C0C1C2[:1]
	C1 := C0C1C2[1 : 1536+1]
	C0C1 := C0C1C2[:1536+1]
	C2 := C0C1C2[1536+1:]

	S0S1S2 := C0C1C2S0S1S2[1536*2+1:]
	S0 := S0S1S2[:1]
	S1 := S0S1S2[1 : 1536+1]
	S2 := S0S1S2[1536+1:]

	if _, err = io.ReadFull(s.Conn, C0C1); err != nil {
		log.Println(err)
		return
	}

	// 0x03 rtmp协议版本号, 明文; 0x06 密文;
	if C0[0] != 3 {
		err = fmt.Errorf("invalid client rtmp version %d", C0[0])
		log.Println(err)
		return
	}
	S0[0] = 3

	cZero := ByteToUint32(C1[4:8], BE)
	//cZero := ByteToUint32BE(C1[4:8])
	log.Printf("cZero: 0x%x", cZero)
	if cZero == 0 {
		log.Println("rtmp simple handshake")
		copy(S1, C2)
		copy(S2, C1)
	} else {
		log.Println("rtmp complex handshake")
		err = CreateComplexS1S2(C1, S1, S2)
		if err != nil {
			log.Println(err)
			return
		}
	}

	if _, err = s.Conn.Write(S0S1S2); err != nil {
		log.Println(err)
		return
	}

	if _, err = io.ReadFull(s.Conn, C2); err != nil {
		log.Println(err)
		return
	}
	return
}

// C1(1536): time(4) + zero(4) + randomByte(1528)
// randomByte(1528): key(764) + digest(764) or digest(764) + key(764)
// key(764): randomData(n) + keyData(128) + randomData(764-n-128-4) + offset(4)
// n = offset的值
// digest(764): offset(4) + randomData(n) + digestData(32) + randomData(764-4-n-32)
// n = offset的值
// 简单握手时c2(1536): time(4) + time(4) + randomEcho(1528)
// 复杂握手时c2(1536): randomData(1504) + digestData(32)
func CreateComplexS1S2(C1, S1, S2 []byte) error {
	// 发起rtmp连接的一方key用FPKeyP，被连接的一方key用FMSKeyP
	// 1 重新计算C1的digest和C1中的digest比较,一样才行
	// 2 计算S1的digest
	// 3 计算S2的key和digest
	var c1Digest []byte
	if c1Digest = DigestFind(C1, 8); c1Digest == nil {
		c1Digest = DigestFind(C1, 772)
	}
	if c1Digest == nil {
		err := fmt.Errorf("can't find digest in C1")
		return err
	}

	cTime := ByteToUint32(C1[0:4], BE)
	cZero := ByteToUint32(C1[4:8], BE)
	sTime := cTime
	sZero := cZero
	//sZero := uint32(0x0d0e0a0d)

	Uint32ToByte(sTime, S1[0:4], BE)
	Uint32ToByte(sZero, S1[4:8], BE)
	rand.Read(S1[8:])
	pos := DigestFindPos(S1, 8)
	s1Digest := DigestCreate(S1, pos, FMSKeyP)
	log.Println("s1Digest create:", s1Digest)
	copy(S1[pos:], s1Digest)

	// FIXME
	s2DigestKey := DigestCreate(c1Digest, -1, FMSKey)
	log.Println("s2DigestKey create:", s2DigestKey)

	rand.Read(S2)
	pos = len(S2) - 32
	s2Digest := DigestCreate(S2, pos, s2DigestKey)
	log.Println("s2Digest create:", s2Digest)
	copy(S2[pos:], s2Digest)
	return nil
}

func DigestFindPos(C1 []byte, base int) (pos int) {
	pos, offset := 0, 0
	for i := 0; i < 4; i++ {
		offset += int(C1[base+i])
	}
	//764 - 4 - 32 = 728, offset 最大值不能超过728
	pos = base + 4 + (offset % 728)
	return
}

func DigestCreate(b []byte, pos int, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	if pos <= 0 {
		h.Write(b)
	} else {
		h.Write(b[:pos])
		h.Write(b[pos+32:])
	}
	return h.Sum(nil)
}

func DigestFind(C1 []byte, base int) []byte {
	pos := DigestFindPos(C1, base)
	c1Digest := DigestCreate(C1, pos, FPKeyP)
	log.Println("c1Digest in C1:", C1[pos:pos+32])
	log.Println("c1Digest create:", c1Digest)
	if hmac.Equal(C1[pos:pos+32], c1Digest) {
		return c1Digest
	}
	return nil
}

/////////////////////////////////////////////////////////////////
// rtmp handle message
/////////////////////////////////////////////////////////////////
// rtmp发送数据的时候 message 拆分成 chunk, MessageSplit()
// rtmp接收数据的时候 chunk 组合成 message, MessageMerge()
// 接收完数据 要对数据处理, MessageHandle()
func RtmpHandleMessage(s *Stream) (err error) {
	var i uint32
	c := &Chunk{}
	for {
		log.Println("==============================>>>", i)
		i++

		if _, err = MessageMerge(s, c); err != nil {
			log.Println(err)
			return
		}
		if err = MessageHandle(s, c); err != nil {
			log.Println(err)
			return
		}

		SendAckMessage(s, c.MsgLength)
		if s.MessageHandleDone {
			log.Println("MessageHandleDone")
			break
		}
	}
	return nil
}

func MessageHandle(s *Stream, c *Chunk) error {
	// rtmp 消息类型: 协议控制消息(1, 2, 3, 5, 6), 命令消息(20, 17),
	// 数据消息(18, 15), 共享对象消息(19, 16), 音频消息(8), 视频消息(9),
	// 聚合消息(22), 用户控制消息(4)
	switch c.MsgTypeId {
	case MsgTypeIdSetChunkSize:
		// 取值范围是 2的31次方 [1-2147483647]
		s.ChunkSize = ByteToUint32(c.MsgData, BE)
		s.RemoteChunkSize = s.ChunkSize
		log.Println("MsgTypeIdSetChunkSize", s.ChunkSize)
	case MsgTypeIdUserControl:
		log.Println("MsgTypeIdUserControl")
	case MsgTypeIdWindowAckSize:
		s.WindowAckSize = ByteToUint32(c.MsgData, BE)
		s.RemoteWindowAckSize = s.WindowAckSize
		log.Println("MsgTypeIdWindowAckSize", s.WindowAckSize)
	case MsgTypeIdDataAmf0, MsgTypeIdShareAmf0, MsgTypeIdCmdAmf0:
		if err := AmfHandle(s, c); err != nil {
			log.Println(err)
			return err
		}
	default:
		err := fmt.Errorf("Untreated MsgTypeId %d", c.MsgTypeId)
		log.Println(err)
		return err
	}
	return nil
}

func SendAckMessage(s *Stream, MsgLen uint32) {
	s.RecvMsgLen += MsgLen
	if s.RecvMsgLen >= s.RemoteWindowAckSize {
		d := Uint32ToByte(s.RecvMsgLen, nil, BE)
		rc := CreateMessage(MsgTypeIdAck, 4, d)
		MessageSplit(s, &rc)
		s.RecvMsgLen = 0
	}
}

/////////////////////////////////////////////////////////////////
// message merge
/////////////////////////////////////////////////////////////////
func MessageMerge(s *Stream, c *Chunk) (Chunk, error) {
	var bh, fmt, csid uint32 // basic header
	for {
		bh, _ = ReadUint32(s.Conn, 1, BE)
		fmt = bh >> 6
		csid = bh & 0x3f // [0, 63]

		// csid: 0表示2字节形式, 1表示3字节形式, 2用于协议控制消息和命令消息, 3-65599表示块流id
		switch csid {
		case 0:
			id, _ := ReadUint32(s.Conn, 1, BE) // [0, 255]
			csid = id + 64                     // [64, 319]
		case 1:
			id, _ := ReadUint32(s.Conn, 2, BE) // [0, 65535]
			csid = id + 64                     // [64, 65599]
		}
		log.Println("fmt:", fmt, "csid:", csid)

		// 一个rtmp连接 可以发送很多流, 一般情况 就一个流
		// 一路流里 可以有 多种数据类型，音频 视频 命令 等
		// FIXME: csid 用于区分流, MsgTypeId 用于区分数据
		sc, ok := s.Chunks[csid]
		if !ok {
			sc = Chunk{}
		}

		sc.Fmt = fmt
		sc.Csid = csid
		if err := ChunkAssemble(s, &sc); err != nil {
			log.Println(err)
			return sc, err
		}

		s.Chunks[csid] = sc
		if sc.Full {
			log.Println("chunk Full")
			if c != nil {
				*c = sc
			}
			return sc, nil
		}
	}
}

func ChunkAssemble(s *Stream, c *Chunk) error {
	// fmt: 控制Message Header的类型, 0表示11字节, 1表示7字节, 2表示3字节, 3表示0字节
	// 音频的fmt 顺序 一般是 0 2 3 3 3 3 3 3 3 3 3
	// 视频的fmt 顺序 一般是 0 3 3 3 1 3 3 3 1 3 3
	switch c.Fmt {
	case 0:
		c.Timestamp, _ = ReadUint32(s.Conn, 3, BE)
		c.MsgLength, _ = ReadUint32(s.Conn, 3, BE)
		c.MsgTypeId, _ = ReadUint32(s.Conn, 1, BE)
		c.MsgStreamId, _ = ReadUint32(s.Conn, 4, LE)
		if c.Timestamp == 0xffffff {
			c.Timestamp, _ = ReadUint32(s.Conn, 4, BE)
			c.TimeExted = true
		} else {
			c.TimeExted = false
		}

		c.MsgData = make([]byte, c.MsgLength)
		c.MsgIndex = 0
		c.MsgRemain = c.MsgLength
		c.Full = false
	case 1:
		c.TimeDelta, _ = ReadUint32(s.Conn, 3, BE)
		c.MsgLength, _ = ReadUint32(s.Conn, 3, BE)
		c.MsgTypeId, _ = ReadUint32(s.Conn, 1, BE)
		if c.TimeDelta == 0xffffff {
			c.Timestamp, _ = ReadUint32(s.Conn, 4, BE)
			c.TimeExted = true
		} else {
			c.TimeExted = false
		}
		c.Timestamp += c.TimeDelta

		c.MsgData = make([]byte, c.MsgLength)
		c.MsgIndex = 0
		c.MsgRemain = c.MsgLength
		c.Full = false
	case 2:
		c.TimeDelta, _ = ReadUint32(s.Conn, 3, BE)
		if c.TimeDelta == 0xffffff {
			c.Timestamp, _ = ReadUint32(s.Conn, 4, BE)
			c.TimeExted = true
		} else {
			c.TimeExted = false
		}
		c.Timestamp += c.TimeDelta
	case 3:
	default:
		return fmt.Errorf("Invalid fmt=%d", c.Fmt)
	}

	size := c.MsgRemain
	if size > s.ChunkSize {
		size = s.ChunkSize
	}

	buf := c.MsgData[c.MsgIndex : c.MsgIndex+size]
	if _, err := s.Conn.Read(buf); err != nil {
		log.Println(err)
		return err
	}
	c.MsgIndex += size
	c.MsgRemain -= size
	if c.MsgRemain == 0 {
		c.Full = true
		// 为了不打印大量的音视频数据
		d := c.MsgData
		c.MsgData = nil
		log.Printf("%#v", c)
		c.MsgData = d
	}
	return nil
}

/////////////////////////////////////////////////////////////////
// message split
/////////////////////////////////////////////////////////////////
func ChunkHeaderAssemble(s *Stream, c *Chunk) error {
	bh := c.Fmt << 6
	switch {
	case c.Csid < 64:
		bh |= c.Csid
		WriteUint32(s.Conn, BE, bh, 1)
	case c.Csid-64 < 320:
		bh |= 0
		WriteUint32(s.Conn, BE, bh, 1)
		WriteUint32(s.Conn, BE, c.Csid-64, 1)
	case c.Csid-64 < 65600:
		bh |= 1
		WriteUint32(s.Conn, BE, bh, 1)
		WriteUint32(s.Conn, BE, c.Csid-64, 2)
	}

	// fmt: 控制Message Header的类型, 0表示11字节, 1表示7字节, 2表示3字节, 3表示0字节
	// csid: 0表示2字节形式, 1表示3字节形式, 2用于协议控制消息和命令消息, 3-65599表示块流id
	if c.Fmt == 3 {
		goto END
	}

	// 至少是3字节
	if c.Timestamp > 0xffffff {
		WriteUint32(s.Conn, BE, 0xffffff, 3)
	} else {
		WriteUint32(s.Conn, BE, c.Timestamp, 3)
	}
	if c.Fmt == 2 {
		goto END
	}

	// 至少是7字节
	WriteUint32(s.Conn, BE, c.MsgLength, 3)
	WriteUint32(s.Conn, BE, c.MsgTypeId, 1)
	if c.Fmt == 1 {
		goto END
	}

	// 就是11字节, 协议文档说StreamId用小端字节序
	WriteUint32(s.Conn, LE, c.MsgStreamId, 4)
END:
	if c.Timestamp > 0xffffff {
		WriteUint32(s.Conn, BE, c.Timestamp, 4)
	}
	return nil
}

func MessageSplit(s *Stream, c *Chunk) error {
	var i, si, ei, div, sLen uint32
	n := c.MsgLength/s.ChunkSize + 1
	log.Println(c.MsgLength, s.ChunkSize, n)

	for i = 0; i < n; i++ {
		if i != 0 {
			c.Fmt = 3
		}

		// send chunk header
		if err := ChunkHeaderAssemble(s, c); err != nil {
			log.Println(err)
			return err
		}

		// send chunk body, 每次发送大小不能超过 chunksize
		si = i * s.ChunkSize
		div = uint32(len(c.MsgData)) - si
		if div > s.ChunkSize {
			ei = si + s.ChunkSize
			sLen += s.ChunkSize
		} else {
			ei = si + div
			sLen += div
		}
		buf := c.MsgData[si:ei]
		if _, err := s.Conn.Write(buf); err != nil {
			log.Println(err)
			return err
		}

		if sLen >= c.MsgLength {
			log.Println("Message send over")
			break
		}
	}
	return nil
}
