package main

import (
	"fmt"
	"io"
	"log"
	"net"
)

type Stream struct {
	Conn                net.Conn
	ChunkSize           uint32
	WindowAckSize       uint32
	RemoteChunkSize     uint32
	RemoteWindowAckSize uint32
	//Chunks              map[uint32]Chunk
	//HandleMessageDone   bool
	IsPublisher   bool
	TransmitStart bool
	//received            uint32
	//ackReceived         uint32
	StreamKey string // Domain + App + StreamName
	//Publisher           RtmpPublisher
	//Players             []RtmpPlayer
	//AmfInfo
	//MediaInfo
	//Statistics
}

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
	}
	return s
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

	cZero := ByteToUint32BE(C1[4:8])
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
