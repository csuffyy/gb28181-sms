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

func RtmpServer() {
	log.Println("start rtmp listen", conf.RtmpListen)
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
		RtmpPublisher(s)
	} else {
		log.Println("player")
		RtmpPlayer(s)
	}
}

func NewStream(c net.Conn) *Stream {
	s := &Stream{
		Conn:                c,
		ChunkSize:           128,
		WindowAckSize:       2500000,
		RemoteChunkSize:     128,
		RemoteWindowAckSize: 2500000,
		//Chunks:              make(map[uint32]Chunk),
	}
	return s
}

// 以小端方式写，以小端方式读
// 以大端方式写，以大端方式读
// 0x11223344	0x11 为高位，0x44为低位
// b[4]			b[0] 为低地址, b[3] 为高地址
// uint32		b[0] b[1] b[2] b[3]
// 0x11223344	0x44 0x33 0x22 0x11 小端 低位在低地址,低地址放低位
// 0x11223344	0x11 0x22 0x33 0x44 大端 低位在高地址,低地址放高位

// /usr/local/go/src/encoding/binary/binary.go
func BeByteToUint32(b []byte) uint32 {
	ui32 := uint32(0)
	for i := 0; i < len(b); i++ {
		ui32 = ui32<<8 + uint32(b[i])
	}
	return ui32
}

func BeUint32ToByte(ui32 uint32, b []byte) {
	for i := 0; i < 4; i++ {
		b[i] = byte(ui32 >> (3 - i) * 8)
	}
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

	cTime := BeByteToUint32(C1[0:4])
	cZero := BeByteToUint32(C1[4:8])
	sTime := cTime
	sZero := cZero
	//sZero := uint32(0x0d0e0a0d)

	BeUint32ToByte(sTime, S1[0:4])
	BeUint32ToByte(sZero, S1[4:8])
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

	cZero := BeByteToUint32(C1[4:8])
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

// rtmp发送数据的时候 message 拆分成 chunk, MessageSplit()
// rtmp接收数据的时候 chunk 组合成 message, ChunkMerge()
func RtmpHandleMessage(s *Stream) (err error) {
	msg := &Chunk{}
	for {
		if err = ChunkMerge(s, msg); err != nil {
			log.Println(err)
			return
		}

		if err = HandleMessage(&msg); err != nil {
			log.Println(err)
			return
		}

		SendAckMessage(msg.Length)

		if s.HandleMessageDone {
			log.Println("HandleMessageDone")
			break
		}
	}
	return nil
}

func RtmpPublisher(s *Stream) {
	if _, ok := Streams[s.StreamKey]; ok {
		log.Println(s.StreamKey, "published")
	} else {
		sp := &StreamPublisher{Type: "rtmp", Publisher: s, Players: make(map[string]*Stream)}
		Streams[s.StreamKey] = sp
		RtmpTransmitStart(sp)
	}
}

type Packet struct {
	IsAudio    bool
	IsVideo    bool
	IsMetadata bool
	StreamID   uint32
	Timestamp  uint32 // dts
	Data       []byte
	//Header     PacketHeader
}

func RtmpRecvData(s *Stream, p *Packet) error {
	return nil
}

func RtmpCacheGOP(s *Stream, p *Packet) {
}

func RtmpTransmitStart(s *StreamPublisher) {
	s.TransmitStart = true

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
