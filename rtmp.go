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
	"os"
	"utils"
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
)

// 新来的rtmp连接 无法确定角色，确定后 日志文件 需重命名
// Stream_Timestamp.log(时间戳到毫秒) 重命名为 Publisher_live_cctv1.log
// Stream_Timestamp.log(时间戳到毫秒) 重命名为 Player_live_cctv1_ip_port.log
// /usr/local/sms/sms			执行程序
// /usr/local/sms/sms.json		配置文件
// /usr/local/sms/sms.log		程序日志文件
// /usr/local/sms/log/Stream_Timestamp.log.log	角色未确定时的日志
// /usr/local/sms/log/live_cctv1/Publisher_live_cctv1.log		发布者日志
// /usr/local/sms/log/live_cctv1/Player_live_cctv1_ip_port.log	播放者日志
// /usr/local/sms/hls/live_cctv1/cctv1.m3u8
// /usr/local/sms/hls/live_cctv1/cctv1_001.ts
// m3u8和ts的存储路径
// HlsDisk/live_cctv1/live_cctv1.m3u8
// HlsDisk/live_cctv1/live_cctv1_12345.ts
type Stream struct {
	Key                 string
	StreamType          string      // rtmpPublisher/ rtmpPlayer/ flvPlayer
	LogFilename         string      // Stream_Timestamp.log
	log                 *log.Logger // 每个发布者、播放者的日志都是独立的
	Conn                net.Conn
	RemoteAddr          string
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
	DataChan            chan *Chunk        // 发布者和播放者的数据通道, 有缓存的
	HlsChan             chan *Chunk        // 发布者和hls生产者的数据通道
	GopCache
	HlsInfo
}

func NewStream(c net.Conn) *Stream {
	s := &Stream{
		LogFilename:         GetTempLogFilename(),
		Conn:                c,
		ChunkSize:           128,
		WindowAckSize:       2500000,
		RemoteChunkSize:     128,
		RemoteWindowAckSize: 2500000,
		Chunks:              make(map[uint32]Chunk),
		Players:             make(map[string]*Stream),
		NewPlayer:           true,
		DataChan:            make(chan *Chunk, 5),
		HlsChan:             make(chan *Chunk, 5),
		GopCache:            GopCacheNew(),
	}
	s.log, _ = StreamLogCreate(s.LogFilename)
	return s
}

/**********************************************************/
/* log
/**********************************************************/
func GetTempLogFilename() string {
	ts := utils.GetTimestamp("ms")
	s := fmt.Sprintf("Stream_%d.log", ts)
	log.Printf("stream tmp LogFile is %s", s)
	return s
}

func StreamLogCreate(fn string) (*log.Logger, error) {
	f, err := os.Create(fn)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	l := log.New(f, "", log.LstdFlags|log.Lshortfile)
	return l, nil
}

func StreamLogRename(s *Stream, sType string) error {
	// 1 创建live_cctv1文件夹
	// 2 日志文件重命名
	// Stream_Timestamp.log 重命名为 cctv1_publisher.log
	// Stream_Timestamp.log 重命名为 cctv1_player_ip_port.log
	var folder, fn string
	if s.IsPublisher {
		folder = fmt.Sprintf("%s_%s", s.AmfInfo.App, s.AmfInfo.PublishName)
		fn = fmt.Sprintf("%s/%s_publisher_%s_%s.log", folder, folder, sType, s.RemoteAddr)
	} else {
		folder = fmt.Sprintf("%s_%s", s.AmfInfo.App, s.AmfInfo.StreamName)
		fn = fmt.Sprintf("%s/%s_player_%s_%s.log", folder, folder, sType, s.RemoteAddr)
	}

	err := os.MkdirAll(folder, 0755)
	if err != nil {
		log.Println(err)
		return err
	}

	// 文件打开状态下 也可以重命名
	err = os.Rename(s.LogFilename, fn)
	if err != nil {
		log.Println(err)
		return err
	}
	log.Printf("%s rename to %s", s.LogFilename, fn)

	s.LogFilename = fn
	return nil
}

/**********************************************************/
/* rtmp connect
/**********************************************************/
// C1(1536): time(4) + zero(4) + randomByte(1528)
// randomByte(1528): key(764) + digest(764) or digest(764) + key(764)
// key(764): randomData(n) + keyData(128) + randomData(764-n-128-4) + offset(4)
// n = offset的值
// digest(764): offset(4) + randomData(n) + digestData(32) + randomData(764-4-n-32)
// n = offset的值
// 简单握手时c2(1536): time(4) + time(4) + randomEcho(1528)
// 复杂握手时c2(1536): randomData(1504) + digestData(32)
func CreateComplexS1S2(s *Stream, C1, S1, S2 []byte) error {
	// 发起rtmp连接的一方key用FPKeyP，被连接的一方key用FMSKeyP
	// 1 重新计算C1的digest和C1中的digest比较,一样才行
	// 2 计算S1的digest
	// 3 计算S2的key和digest
	var c1Digest []byte
	if c1Digest = DigestFind(s, C1, 8); c1Digest == nil {
		c1Digest = DigestFind(s, C1, 772)
	}
	if c1Digest == nil {
		err := fmt.Errorf("can't find digest in C1")
		s.log.Println(err)
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
	s.log.Println("s1Digest create:", s1Digest)
	copy(S1[pos:], s1Digest)

	// FIXME
	s2DigestKey := DigestCreate(c1Digest, -1, FMSKey)
	s.log.Println("s2DigestKey create:", s2DigestKey)

	rand.Read(S2)
	pos = len(S2) - 32
	s2Digest := DigestCreate(S2, pos, s2DigestKey)
	s.log.Println("s2Digest create:", s2Digest)
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

func DigestFind(s *Stream, C1 []byte, base int) []byte {
	pos := DigestFindPos(C1, base)
	c1Digest := DigestCreate(C1, pos, FPKeyP)
	s.log.Println("c1Digest in C1:", C1[pos:pos+32])
	s.log.Println("c1Digest create:", c1Digest)
	if hmac.Equal(C1[pos:pos+32], c1Digest) {
		return c1Digest
	}
	return nil
}

func RtmpHandshakeServer(s *Stream) (err error) {
	var C0C1C2S0S1S2 [(1 + 1536*2) * 2]byte

	C0C1C2 := C0C1C2S0S1S2[:1536*2+1]
	//C0 := C0C1C2[:1]
	C1 := C0C1C2[1 : 1536+1]
	//C0C1 := C0C1C2[:1536+1]
	C2 := C0C1C2[1536+1:]

	S0S1S2 := C0C1C2S0S1S2[1536*2+1:]
	S0 := S0S1S2[:1]
	S1 := S0S1S2[1 : 1536+1]
	S2 := S0S1S2[1536+1:]

	//if _, err = io.ReadFull(s.Conn, C0C1); err != nil {
	if _, err = io.ReadFull(s.Conn, C1); err != nil {
		s.log.Println(err)
		return
	}

	/*
		// 0x03 rtmp协议版本号, 明文; 0x06 密文;
		if C0[0] != 3 {
			err = fmt.Errorf("invalid rtmp client version %d", C0[0])
			s.log.Println(err)
			return
		}
	*/
	S0[0] = 3

	cZero := ByteToUint32(C1[4:8], BE)
	//cZero := ByteToUint32BE(C1[4:8])
	s.log.Printf("cZero: 0x%x", cZero)
	if cZero == 0 {
		s.log.Println("rtmp simple handshake")
		copy(S1, C2)
		copy(S2, C1)
	} else {
		s.log.Println("rtmp complex handshake")
		err = CreateComplexS1S2(s, C1, S1, S2)
		if err != nil {
			s.log.Println(err)
			return
		}
	}

	if _, err = s.Conn.Write(S0S1S2); err != nil {
		s.log.Println(err)
		return
	}

	if _, err = io.ReadFull(s.Conn, C2); err != nil {
		s.log.Println(err)
		return
	}
	return
}

/**********************************************************/
/* rtmp chunk and message handle
/**********************************************************/
// DataType    string
// "Metadata", "VideoHeader", "AudioHeader",
// "VideoKeyFrame", "VideoInterFrame", "AudioAacFrame"
type Chunk struct {
	FmtFirst    uint32 // 2bit, 发送的时候要用
	Fmt         uint32 // 2bit, format
	Csid        uint32 // 6/14/22bit, chunk stream id
	Timestamp   uint32 // 24bit
	TimeExted   bool   // 8bit
	TimeDelta   uint32 // 24bit
	MsgLength   uint32 // 24bit
	MsgTypeId   uint32 // 8bit
	MsgStreamId uint32 // 32bit, 小端字节序
	MsgData     []byte
	MsgIndex    uint32 // 接收的数据 写到MsgData的哪里
	MsgRemain   uint32 // MsgData 还有多少数据需要接收
	Full        bool   // 8bit
	DataType    string
}

// rtmp发送数据的时候 message 拆分成 chunk, MessageSplit()
// rtmp接收数据的时候 chunk 组合成 message, MessageMerge()
// 接收完数据 要对数据处理, MessageHandle()
func RtmpHandleMessage(s *Stream) (err error) {
	var i uint32
	c := &Chunk{}
	for {
		s.log.Println("====================>> message", i)
		i++

		if _, err = MessageMerge(s, c); err != nil {
			s.log.Println(err)
			return
		}
		if err = MessageHandle(s, c); err != nil {
			s.log.Println(err)
			return
		}

		SendAckMessage(s, c.MsgLength)
		if s.MessageHandleDone {
			s.log.Println("MessageHandleDone")
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
		s.log.Println("MsgTypeIdSetChunkSize", s.ChunkSize)
	case MsgTypeIdUserControl:
		s.log.Println("MsgTypeIdUserControl")
	case MsgTypeIdWindowAckSize:
		s.WindowAckSize = ByteToUint32(c.MsgData, BE)
		s.RemoteWindowAckSize = s.WindowAckSize
		s.log.Println("MsgTypeIdWindowAckSize", s.WindowAckSize)
	case MsgTypeIdDataAmf0, MsgTypeIdShareAmf0, MsgTypeIdCmdAmf0:
		if err := AmfHandle(s, c); err != nil {
			s.log.Println(err)
			return err
		}
	default:
		err := fmt.Errorf("Untreated MsgTypeId %d", c.MsgTypeId)
		s.log.Println(err)
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

func MessageMerge(s *Stream, c *Chunk) (Chunk, error) {
	var bh, fmt, csid uint32 // basic header
	var err error
	var sc Chunk
	i := 0
	for {
		s.log.Println("-------------------->> chunk", i)
		bh, err = ReadUint32(s.Conn, 1, BE)
		if err != nil {
			s.log.Println(err)
			return sc, err
		}
		fmt = bh >> 6
		csid = bh & 0x3f // [0, 63]
		// csid 6bit,  表示范围[0, 63],     0 1 2 有特殊用处
		// csid 8bit,  表示范围[64, 319],   [0, 255]+64
		// csid 16bit, 表示范围[64, 65599], [0, 65535]+64
		// csid 应该优先使用最小字节表示,   [320, 65599]
		// csid 0表示2字节形式, csid 1表示3字节形式
		// csid 2用于协议的控制消息和命令消息
		// csid [3, 65599]表示块流id, 共65597个
		switch csid {
		case 0:
			id, _ := ReadUint32(s.Conn, 1, BE) // [0, 255]
			csid = id + 64                     // [64, 319]
		case 1:
			id, _ := ReadUint32(s.Conn, 2, BE) // [0, 65535]
			csid = id + 64                     // [64, 65599]
		}
		s.log.Println("fmt:", fmt, "csid:", csid)

		// 一个rtmp连接 可以发送很多流, 一般情况 就一个流
		// 一路流里 可以有 多种数据类型，音频 视频 命令 等
		// FIXME: csid 用于区分流, MsgTypeId 用于区分数据
		sc, ok := s.Chunks[csid]
		if !ok {
			sc = Chunk{}
		}

		if i == 0 {
			sc.FmtFirst = fmt
		}
		i++

		sc.Fmt = fmt
		sc.Csid = csid
		if err := ChunkAssemble(s, &sc); err != nil {
			s.log.Println(err)
			return sc, err
		}

		s.Chunks[csid] = sc
		if sc.Full {
			s.log.Println("chunk Full")
			if c != nil {
				*c = sc
			}
			return sc, nil
		}
	}
}

// fmt: 控制Message Header的类型, 0表示11字节, 1表示7字节, 2表示3字节, 3表示0字节
// 音频的fmt顺序 一般是0 2 3 3 3 3 3 3 3 3 3 //理想状态 数据量和时间增量相同
// 音频的fmt顺序 一般是0 1 1 1 2 1 3 1 1 2 1 //实际情况 数据量偶尔同 时间增量偶尔同
// 视频的fmt顺序 一般是0 3 3 3 1 1 3 3 1 3 3 //理想状态和实际情况一样
// 块大小默认是128, 通常会设置为1024, 音频消息约400字节, 视频消息约700-30000字节
func ChunkAssemble(s *Stream, c *Chunk) error {
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
		s.log.Printf("Timestamp=%d, MsgLength=%d, MsgTypeId=%d, MsgStreamId=%d", c.Timestamp, c.MsgLength, c.MsgTypeId, c.MsgStreamId)

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
		s.log.Printf("TimeDelta=%d, Timestamp=%d, MsgLength=%d, MsgTypeId=%d, MsgStreamId=%d", c.TimeDelta, c.Timestamp, c.MsgLength, c.MsgTypeId, c.MsgStreamId)

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
		s.log.Printf("TimeDelta=%d, Timestamp=%d, MsgLength=%d, MsgTypeId=%d, MsgStreamId=%d", c.TimeDelta, c.Timestamp, c.MsgLength, c.MsgTypeId, c.MsgStreamId)

		c.MsgIndex = 0
		c.MsgRemain = c.MsgLength
		c.Full = false
	case 3:
		// TODO: 有可能有4字节的扩展时间戳
		if c.TimeExted == true {
			//c.Timestamp, _ = ReadUint32(s.Conn, 4, BE)
		}

		if c.MsgRemain == 0 {
			c.Timestamp += c.TimeDelta

			c.MsgIndex = 0
			c.MsgRemain = c.MsgLength
			c.Full = false
		}
	default:
		return fmt.Errorf("Invalid fmt=%d", c.Fmt)
	}

	size := c.MsgRemain
	if size > s.ChunkSize {
		size = s.ChunkSize
	}
	s.log.Printf("read data size is %d", size)

	buf := c.MsgData[c.MsgIndex : c.MsgIndex+size]
	if _, err := s.Conn.Read(buf); err != nil {
		s.log.Println(err)
		return err
	}
	c.MsgIndex += size
	c.MsgRemain -= size
	if c.MsgRemain == 0 {
		c.Full = true
		// 为了不打印大量的音视频数据
		d := c.MsgData
		c.MsgData = nil
		s.log.Printf("%#v", c)
		c.MsgData = d
	}
	return nil
}

func ChunkHeaderAssemble(s *Stream, c *Chunk) error {
	var err error
	bh := c.Fmt << 6
	switch {
	case c.Csid < 64:
		bh |= c.Csid
		err = WriteUint32(s.Conn, BE, bh, 1)
	case c.Csid-64 < 320:
		bh |= 0
		err = WriteUint32(s.Conn, BE, bh, 1)
		err = WriteUint32(s.Conn, BE, c.Csid-64, 1)
	case c.Csid-64 < 65600:
		bh |= 1
		err = WriteUint32(s.Conn, BE, bh, 1)
		err = WriteUint32(s.Conn, BE, c.Csid-64, 2)
	}
	if err != nil {
		s.log.Println(err)
		return err
	}

	// fmt: 控制Message Header的类型, 0表示11字节, 1表示7字节, 2表示3字节, 3表示0字节
	// csid: 0表示2字节形式, 1表示3字节形式, 2用于协议控制消息和命令消息, 3-65599表示块流id
	if c.Fmt == 3 {
		goto END
	}

	// 至少是3字节
	if c.Timestamp > 0xffffff {
		err = WriteUint32(s.Conn, BE, 0xffffff, 3)
	} else {
		err = WriteUint32(s.Conn, BE, c.Timestamp, 3)
	}
	if err != nil {
		s.log.Println(err)
		return err
	}

	if c.Fmt == 2 {
		goto END
	}

	// 至少是7字节
	err = WriteUint32(s.Conn, BE, c.MsgLength, 3)
	err = WriteUint32(s.Conn, BE, c.MsgTypeId, 1)
	if err != nil {
		s.log.Println(err)
		return err
	}

	if c.Fmt == 1 {
		goto END
	}

	// 就是11字节, 协议文档说StreamId用小端字节序
	err = WriteUint32(s.Conn, LE, c.MsgStreamId, 4)
	if err != nil {
		s.log.Println(err)
		return err
	}
END:
	if c.Timestamp > 0xffffff {
		WriteUint32(s.Conn, BE, c.Timestamp, 4)
	}
	return nil
}

func MessageSplit(s *Stream, c *Chunk) error {
	var i, si, ei, div, sLen uint32
	n := c.MsgLength/s.ChunkSize + 1
	s.log.Printf("@@@ send MsgLength=%d, ChunkSize=%d, ChunkNum=%d", c.MsgLength, s.ChunkSize, n)

	c.Fmt = 0
	for i = 0; i < n; i++ {
		if i != 0 {
			c.Fmt = 3
		}
		s.log.Printf("@@@ send fmt=%d, TimeDelta=%d, Timestamp=%d, MsgLength=%d, MsgTypeId=%d, MsgStreamId=%d", c.Fmt, c.TimeDelta, c.Timestamp, c.MsgLength, c.MsgTypeId, c.MsgStreamId)

		// send chunk header
		if err := ChunkHeaderAssemble(s, c); err != nil {
			s.log.Println("@@@ message send error")
			s.log.Println(err)
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
			s.log.Println("@@@ message send error")
			s.log.Println(err)
			return err
		}

		if sLen >= c.MsgLength {
			s.log.Println("@@@ message send ok")
			break
		}
	}
	return nil
}

/**********************************************************/
/* rtmp Server
/**********************************************************/
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
		log.Println("---------->> new tcp connect")
		log.Println("RemoteAddr:", c.RemoteAddr().String())

		ui8, err := ReadUint8(c)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Printf("tcp first byte is %#x", ui8)

		if ui8 == 0xff {
			go FlvPlayer(c)
		} else {
			// 0x03 rtmp协议版本号, 明文; 0x06 密文;
			if ui8 != 3 {
				log.Printf("invalid rtmp client version %d", ui8)
				c.Close()
				continue
			}
			go RtmpHandler(c)
		}
	}
}

func RtmpHandler(c net.Conn) {
	// 这里还无法区分是 rtmp推流 或 rtmp播放
	s := NewStream(c)
	s.RemoteAddr = c.RemoteAddr().String()

	if err := RtmpHandshakeServer(s); err != nil {
		s.log.Println(err)
		return
	}
	s.log.Println("RtmpHandshakeServer ok")

	if err := RtmpHandleMessage(s); err != nil {
		s.log.Println(err)
		return
	}
	s.log.Println("RtmpHandleMessage ok")

	//log.Printf("%#v", s)
	s.log.Printf("the stream have %d chunks", len(s.Chunks))
	s.log.Printf("---> the stream is publisher %t", s.IsPublisher)
	StreamLogRename(s, "rtmp")

	if s.IsPublisher {
		// for循环负责接收数据，发送如何处理 ???
		// 方法1: 发送也在 for循环里做，这样可能会影响接收
		// 方法2: 每个播放一个发送协程, 这样发送协程可能会太多
		// 方法3: 所有播放共用一个发送协程, 进入for循环前 创建一个发送协程
		s.StreamType = "rtmpPublisher"
		RtmpPublisher(s) // 进入for循环 接收数据并拷贝给发送协程
	} else {
		s.StreamType = "rtmpPlayer"
		RtmpPlayer(s) // 只需把播放信息 存入到Publisher的Players里
	}
}

func RtmpPublisher(s *Stream) {
	s.Key = fmt.Sprintf("%s_%s", s.AmfInfo.App,
		s.AmfInfo.PublishName)
	s.log.Println("publisher key is", s.Key)

	_, ok := Publishers[s.Key]
	if ok { // 发布者已存在, 断开当前连接并返回错误
		s.log.Printf("publisher %s is exist", s.Key)
		s.Conn.Close()
		return
	}
	Publishers[s.Key] = s

	// 初始化hls的生产
	folder := fmt.Sprintf("%s/hls", s.Key)
	err := os.MkdirAll(folder, 0755)
	if err != nil {
		log.Println(err)
		return
	}
	s.M3u8Path = fmt.Sprintf("%s/%s.m3u8", folder, s.Key)
	s.log.Println("m3u8Path is", s.M3u8Path)
	s.M3u8File, err = os.OpenFile(s.M3u8Path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		log.Println(err)
		return
	}
	s.HlsInfo.TsList = list.New()

	go HlsCreator(s) // 开启hls生产协程
	go RtmpSender(s) // 给所有播放者发送数据

	s.TransmitSwitch = "on"
	i := 0
	for {
		s.log.Println("====================>> message", i)
		if s.TransmitSwitch == "off" {
			s.log.Printf("publisher %s close", s.Key)
			s.Conn.Close()
			return
		}

		// 接收数据 和 传递数据给发送者
		if err := RtmpReceiver(s); err != nil {
			s.log.Println(err)
			s.log.Println("The connect closed by peer")
			s.Conn.Close()
			return
		}

		// 0 is MetaData;
		// 1 is video AVC sequence header
		// 2 is audio AAC sequence header
		// 0 VideoKeyFrame 24665 0
		// 1 VideoInterFrame 5137 42
		// 2 AudioAacFrame 366 42
		// 3 AudioAacFrame 367 65
		// 4 VideoInterFrame 714 84
		// 5 AudioAacFrame 366 88
		// 6 AudioAacFrame 367 112
		// 7 VideoInterFrame 7998 125
		// 8 AudioAacFrame 366 135
		// 9 AudioAacFrame 540 158
		// 10 VideoInterFrame 9075 167
		// 11 AudioAacFrame 534 181
		// 12 AudioAacFrame 443 205
		// 测试使用, 控制接收 多少个Message
		if i == 10 {
			s.Conn.Close()
			break
		}
		i++
	}
}

func RtmpReceiver(s *Stream) error {
	var c Chunk
	var err error

	// 接收 并 合并 数据
	if c, err = MessageMerge(s, nil); err != nil {
		s.log.Println(err)
		s.log.Println("RtmpReceiver close")
		close(s.DataChan)
		return err
	}
	SendAckMessage(s, c.MsgLength)
	s.log.Printf("Message TypeId %d, len %d", c.MsgTypeId, c.MsgLength)
	//s.log.Printf("%x", c.MsgData)

	if c.MsgTypeId == MsgTypeIdCmdAmf0 { // 20
		AmfHandle(s, &c)
	}
	if c.MsgTypeId == MsgTypeIdAudio { // 8
		AudioHandle(s, &c)
	}
	if c.MsgTypeId == MsgTypeIdVideo { // 9
		VideoHandle(s, &c)
	}
	if c.MsgTypeId == MsgTypeIdDataAmf3 || // 15
		c.MsgTypeId == MsgTypeIdDataAmf0 { // 18
		MetadataHandle(s, &c)
	}

	s.log.Printf("GopCacheMax=%d, GopCacheNum=%d, MediaDataLen=%d", s.GopCacheMax, s.GopCacheNum, s.MediaData.Len())
	PrintList(s, s.MediaData)

	s.DataChan <- &c
	return nil
}

// 启播方式： 默认采用快速启播
// 1 快速启播：先发送缓存的gop数据, 再发送最新数据. 启播快 但延时交高
// 2 低延时启播：直接发送最新数据. 启播交慢 但是延时最低
func RtmpSender(s *Stream) {
	var err error
	for {
		c, ok := <-s.DataChan
		if !ok {
			s.log.Println("RtmpSender close")
			// 释放所有播放者和其他资源
			s.log.Printf("release publisher %s resource", s.Key)
			delete(Publishers, s.Key)
			return
		}
		s.HlsChan <- c // 发送数据给hls生产协程

		s.log.Println("@@@ RtmpSender() start")
		s.log.Printf("@@@ player num is %d, send DataType is %s, size is %d", len(s.Players), c.DataType, c.MsgLength)
		for _, p := range s.Players {
			// 新播放者，先发送缓存的gop数据，再发送最新数据
			// 老播放者，直接发送最新数据
			s.log.Printf("@@@ %s is NewPlayer %t", p.Key, p.NewPlayer)
			if p.NewPlayer == true {
				p.NewPlayer = false
				if p.StreamType == "rtmpPlayer" {
					err = GopCacheSend(p, &s.GopCache)
				} else if p.StreamType == "flvPlayer" {
					err = GopCacheSendFlv(p, &s.GopCache)
				}
			} else {
				if p.StreamType == "rtmpPlayer" {
					err = MessageSplit(p, c)
				} else if p.StreamType == "flvPlayer" {
					err = MessageSendFlv(p, c)
				}
			}

			if err != nil {
				s.log.Println(err)
				s.log.Printf("@@@ send data to player %s error, %t",
					p.Key, p.NewPlayer)
				delete(s.Players, p.Key)
			}
		}
		s.log.Println("@@@ RtmpSender() stop")
	}
}

func RtmpPlayer(s *Stream) {
	key := fmt.Sprintf("%s_%s", s.AmfInfo.App, s.AmfInfo.StreamName)
	s.log.Println("publisher key is", key)

	p, ok := Publishers[key]
	if !ok { // 发布者不存在, 断开连接并返回错误
		s.log.Printf("publisher %s isn't exist", key)
		s.Conn.Close()
		return
	}

	s.Key = fmt.Sprintf("%s_%s_%s", s.AmfInfo.App,
		s.AmfInfo.StreamName, s.RemoteAddr)
	s.log.Println("player key is", s.Key)
	p.Players[s.Key] = s
}

func PrintList(s *Stream, l *list.List) {
	s.log.Println(">>>>>> s.MediaData list <<<<<<")
	i := 0
	for e := l.Front(); e != nil; e = e.Next() {
		v := (e.Value).(*Chunk)
		s.log.Println(i, v.DataType, v.MsgLength, v.Timestamp)
		i++
	}
}

/**********************************************************/
/* Metadata Video Audio handle
/**********************************************************/
// Metadata 数据要缓存起来，发送给播放者
func MetadataHandle(s *Stream, c *Chunk) error {
	c.DataType = "Metadata"
	r := bytes.NewReader(c.MsgData)
	vs, err := AmfUnmarshal(s, r) // 序列化转结构化
	if err != nil && err != io.EOF {
		s.log.Println(err)
		return err
	}
	s.log.Printf("Amf Unmarshal %#v", vs)

	s.GopCache.MetaData = c
	return nil
}

func VideoHandle(s *Stream, c *Chunk) error {
	FrameType := c.MsgData[0] >> 4 // 4bit
	CodecId := c.MsgData[0] & 0xf  // 4bit
	s.log.Printf("FrameType=%d, CodecId=%d", FrameType, CodecId)

	//1: keyframe (for AVC, a seekable frame), 关键帧(I帧)
	//2: inter frame (for AVC, a non-seekable frame), 非关键帧(P/B帧)
	//3: disposable inter frame (H.263 only)
	//4: generated keyframe (reserved for server use only)
	//5: video info/command frame
	if FrameType == 1 {
		s.log.Println("FrameType is KeyFrame(I frame)")
		c.DataType = "VideoKeyFrame"
	} else if FrameType == 2 {
		// 如何区分是 B帧 还是 P帧???
		s.log.Println("FrameType is InterFrame(B/P frame)")
		c.DataType = "VideoInterFrame"
	} else {
		err := fmt.Errorf("untreated FrameType %d", FrameType)
		s.log.Println(err)
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
		s.log.Println(err)
		return err
	}

	//0: AVC sequence header
	//1: AVC NALU
	//2: AVC end of sequence
	AVCPacketType := c.MsgData[1] // 8bit
	//创作时间 int24
	CompositionTime := ByteToInt32(c.MsgData[2:5], BE) // 24bit

	if AVCPacketType == 0 {
		s.log.Println("This frame is AVC sequence header")
		c.DataType = "VideoHeader"

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
		s.log.Printf("%#v", AvcC)

		s.GopCache.VideoHeader = c
	} else if AVCPacketType == 1 {
		// One or more NALUs
		s.log.Println("This frame is AVC NALU")
		c.Fmt = c.FmtFirst
		s.GopCache.MediaData.PushBack(c)

		//KeyFrameNum	1	2	3	4	关键帧个数
		//CacheNum		0	1	2	3	Gop个数 为1的时候要开始删Gop了
		//CacheMax		1	1	1	1	Gop最大个数
		if FrameType == 1 {
			if s.GopCache.MediaData.Len() > 1 {
				s.GopCache.GopCacheNum++
			}
			// 这里触发更新 CacheData
			GopCacheUpdate(s)
		}
	} else {
		// Empty
		s.log.Println("This frame is AVC end of sequence")
	}

	s.log.Printf("AVCPacketType=%d, Composition=%d, DataLen=%d",
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

// >>> SoundFormat <<<
//0 = Linear PCM, platform endian
//1 = ADPCM
//2 = MP3
//3 = Linear PCM, little endian
//4 = Nellymoser 16-kHz mono
//5 = Nellymoser 8-kHz mono
//6 = Nellymoser
//7 = G.711 A-law logarithmic PCM
//8 = G.711 mu-law logarithmic PCM
//9 = reserved
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
		s.log.Println("SoundFormat is AAC")
	} else {
		err := fmt.Errorf("untreated SoundFormat %d", SoundFormat)
		s.log.Println(err)
		return err
	}

	//0: AAC sequence header
	//1: AAC raw
	AACPacketType := c.MsgData[1]
	// 10, 3, 1, 1, 0/1
	s.log.Println(SoundFormat, SoundRate, SoundSize, SoundType, AACPacketType)

	if AACPacketType == 0 {
		s.log.Println("This frame is AAC sequence header")
		c.DataType = "AudioHeader"

		//0xaf 0x00 0x12 0x10
		//0101 11 1 1, 00000000, 00010 0100 0010 0 0 0
		//AudioSpecificConfig is explained in ISO 14496-3
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

		s.GopCache.AudioHeader = c
	} else {
		// Raw AAC frame data
		s.log.Println("This frame is AAC raw")
		c.DataType = "AudioAacFrame"
		c.Fmt = c.FmtFirst
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

/**********************************************************/
/* GopCache
/**********************************************************/
// 要缓存的数据
// 1 Metadata
// 2 Video Header
// 3 Audio Header
// 4 MediaData 里面有 I/B/P帧和音频帧, 按来的顺序存放，
//   MediaData 里内容举例：I B P B A A B P I ...
//MediaData里 最多有 GopCacheMax 个 Gop的数据
//比如GopCacheMax=2, 那么MediaData里最多有2个Gop, 第2个Gop不完整, 这样做发送时方便
//当第3个Gop的关键帧到达的时，删除第1个Gop的数据
type GopCache struct {
	GopCacheMax int // 最多缓存几个Gop, 默认为1个
	GopCacheNum int // VideoData里 I帧的个数
	MetaData    *Chunk
	VideoHeader *Chunk
	AudioHeader *Chunk
	MediaData   *list.List // 双向链表, FIXME: 写的时候不能读
}

func GopCacheNew() GopCache {
	return GopCache{
		GopCacheMax: 1,
		MediaData:   list.New(),
	}
}

func GopCacheUpdate(s *Stream) {
	// 1 先判断CacheData里的关键帧个数 是否达到GopCacheMax, 如果没有就直接存入并退出
	// 2 如果达到, 就先删除CacheData里最早的Gop(含音频帧), 然后再存入
	gc := s.GopCache
	s.log.Printf("GopCacheMax=%d, GopCacheNum=%d", gc.GopCacheMax, gc.GopCacheNum)
	if gc.GopCacheNum < gc.GopCacheMax {
		return
	}

	KeyFrameNum := 0
	var n *list.Element
	for e := gc.MediaData.Front(); e != nil; e = n {
		v := (e.Value).(*Chunk)
		//s.log.Printf("list show: %s, %d, %d", v.DataType, v.MsgLength, v.Timestamp)
		if v.DataType == "VideoKeyFrame" {
			KeyFrameNum++
		}
		if KeyFrameNum == 2 {
			break
		}
		s.log.Printf("list remove: %s, %d, %d", v.DataType, v.MsgLength, v.Timestamp)
		n = e.Next()
		gc.MediaData.Remove(e)
	}
	s.GopCache.GopCacheNum--
}

func GopCacheSend(s *Stream, gop *GopCache) error {
	// 1 发送Metadata
	s.log.Println("@@@ send Metadata")
	MessageSplit(s, gop.MetaData)
	// 2 发送VideoHeader
	s.log.Println("@@@ send VideoHeader")
	MessageSplit(s, gop.VideoHeader)
	// 3 发送AudioHeader
	s.log.Println("@@@ send AudioHeader")
	MessageSplit(s, gop.AudioHeader)
	// 4 发送MediaData(包含最后收到的数据)
	s.log.Println("@@@ send MediaData")
	s.log.Println("@@@ GopCache.MediaData len", gop.MediaData.Len())
	i := 0
	var err error
	for e := gop.MediaData.Front(); e != nil; e = e.Next() {
		v := (e.Value).(*Chunk)
		err = MessageSplit(s, v)
		if err != nil {
			s.log.Println(err)
			s.log.Println("@@@ GopCacheSend() stop")
			return err
		}
		s.log.Println("@@@ GopCacheSend", i, v.DataType, v.MsgLength, v.Timestamp)
		i++
	}
	s.log.Println("@@@ GopCacheSend() ok")
	return nil
}
