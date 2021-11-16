package main

import (
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

type RtmpTask struct {
}

type Stream struct {
	Conn                net.Conn
	ChunkSize           uint32
	WindowAckSize       uint32
	RemoteChunkSize     uint32
	RemoteWindowAckSize uint32
	Chunks              map[uint32]Chunk
	HandleMessageDone   bool
	IsPublisher         bool
	TransmitStart       bool
	//received            uint32
	//ackReceived         uint32
	StreamKey string // Domain + App + StreamName
	//Publisher           RtmpPublisher
	//Players             []RtmpPlayer
	AmfInfo
	//MediaInfo
	//Statistics
	Cache
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

	log.Printf("%#v", s)
	if s.IsPublisher {
		log.Println("publish")
		//RtmpPublisher(s)
	} else {
		log.Println("player")
		//RtmpPlayer(s)
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
	}
	return s
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

		if err = MessageMerge(s, c); err != nil {
			log.Println(err)
			return
		}

		if err = MessageHandle(s, c); err != nil {
			log.Println(err)
			return
		}

		SendAckMessage(c.MsgLength)

		if s.HandleMessageDone {
			log.Println("HandleMessageDone")
			break
		}
	}
	return nil
}

/////////////////////////////////////////////////////////////////
// message merge
/////////////////////////////////////////////////////////////////
func MessageMerge(s *Stream, c *Chunk) error {
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
		// 一路流里 可以有 多数数据类型，音频 视频 命令 等
		// csid 用于区分流, MsgTypeId 用于区分数据
		sc, ok := s.Chunks[csid]
		if !ok {
			sc = Chunk{}
		}

		sc.Fmt = fmt
		sc.Csid = csid
		if err := ChunkAssmble(s, &sc); err != nil {
			log.Println(err)
			return err
		}

		s.Chunks[csid] = sc
		if sc.Full {
			*c = sc
			log.Println("chunk Full")
			break
		}
	}
	return nil
}

func ChunkAssmble(s *Stream, c *Chunk) error {
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
		log.Printf("%#v", c)
	}
	return nil
}

/////////////////////////////////////////////////////////////////
// message handle
/////////////////////////////////////////////////////////////////
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

func SendAckMessage(MsgLen uint32) {
	log.Println(MsgLen)
	/*
		s.received += uint32(len)
		s.ackReceived += uint32(len)
		if s.received >= 0xf0000000 {
			s.received = 0
		}
		if s.ackReceived >= s.remoteWindowAckSize {
			c := CreateMessage(MsgTypeIdAck, 4, s.ackReceived)
			c.ChunkDisAssmble(s.Conn, s.chunkSize)
			s.ackReceived = 0
		}
	*/
}

/////////////////////////////////////////////////////////////////
// message merge
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

//////////////////////////////////////////////////////////////////////////

type Packet struct {
	IsAudio    bool
	IsVideo    bool
	IsMetadata bool
	StreamID   uint32
	Timestamp  uint32 // dts
	Data       []byte
	//Header     PacketHeader
}

type StreamPublisher struct {
	Type      string // rtmp or gb28181
	Publisher *Stream
	Players   map[string]*Stream
}

type StreamPlayer struct {
	Type   string // rtmp or flv
	stream *Stream
}

type Cache struct {
	GopStart     bool
	GopNum       int
	GopCount     int
	GopNextIndex int
	GopIndex     int
	GopPacket    []*Packet
	MetaFull     bool
	MetaPacket   *Packet
	AudioFull    bool
	AudioPacket  *Packet
	VideoFull    bool
	VideoPacket  *Packet
}

func RtmpPublisher(s *Stream) {
	/*
		if _, ok := Streams[s.StreamKey]; ok {
			log.Println(s.StreamKey, "published")
		} else {
			sp := &StreamPublisher{Type: "rtmp", Publisher: s, Players: make(map[string]*Stream)}
			Streams[s.StreamKey] = sp
			RtmpTransmitStart(sp)
		}
	*/
}

func RtmpRecvData(s *Stream, p *Packet) error {
	return nil
}

func RtmpCacheGOP(s *Stream, p *Packet) {
}

func RtmpTransmitStart(s *StreamPublisher) {
	//s.TransmitStart = true

	/*
		var p Packet
		for {
			if !s.TransmitStart {
				s.Conn.Close()
				return
			}

			RtmpRecvData(s, &p)

			// yyy
			s.RtmpCacheGOP(&p)

			//s.RtmpSendData()
			for _, player := range s.Players {
				log.Println(player)
				if !player.Start {
					s.RtmpSendGOP()
					player.Start = true
				} else {
					// send new packet
				}
			}
		}
	*/
}

func RtmpPlayer(s *Stream) {
}
