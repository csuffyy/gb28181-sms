package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type FlvPlayInfo struct {
	App    string
	Stream string
	Client string
}

/**********************************************************/
/* for http
/**********************************************************/
func GetAppStream(str string) (string, string) {
	s := strings.Split(str, "/")
	if len(s) < 3 {
		return "", ""
	}
	ss := strings.Split(s[2], ".")
	if len(ss) < 1 {
		return "", ""
	}
	return s[1], ss[0]
}

// 0xff + len(1 Byte) + data(len Byte)
func FlvRecv(fpi FlvPlayInfo) (net.Conn, error) {
	data, err := json.Marshal(fpi)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	//log.Println(data)

	c, err := net.Dial("tcp", "127.0.0.1:1935")
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = WriteUint8(c, 0xff)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	err = WriteUint8(c, uint8(len(data)))
	if err != nil {
		log.Println(err)
		return nil, err
	}
	_, err = WriteByte(c, data)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return c, nil
}

func FlvSend(c net.Conn, w http.ResponseWriter) {
	buf := make([]byte, 4096)
	var n, m int = 0, 0
	var err error

	defer func() {
		c.Close()
		buf = nil
	}()

	for {
		n, err = c.Read(buf[:])
		//log.Println("flvRead", n, err, len(buf))
		if err != nil || n == 0 {
			log.Println("receive data error.", err)
			break
		}

		m, err = w.Write(buf[:n])
		//log.Println("flvWrite", m, err, len(buf))
		if err != nil || m == 0 {
			log.Println("send data error.", err)
			break
		}
	}
}

// tcp连接到 Publisher 获取数据
// 收到数据后 按http-flv格式 发送数据
// GET http://www.domain.com/live/yuankang.flv
func GetFlv(w http.ResponseWriter, r *http.Request) {
	var fpi FlvPlayInfo
	fpi.App, fpi.Stream = GetAppStream(r.URL.String())
	fpi.Client = r.RemoteAddr
	log.Printf("%#v", fpi)

	c, err := FlvRecv(fpi)
	if err != nil {
		log.Println(err)
		goto ERR
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Content-Type", "video/x-flv")
	w.Header().Set("transfer-encoding", "chunked")
	w.Header().Set("Server", AppName)
	FlvSend(c, w)
	return
ERR:
	rsps := GetRsps(500, err.Error())
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-length", strconv.Itoa(len(rsps)))
	w.Header().Set("Server", AppName)
	w.Write(rsps)
}

/**********************************************************/
/* rtmp2flv
/**********************************************************/
// 1 获取播放信息 app stream client
// 2 查找是否有发布者
// 3 创建Stream 挂在到 Publisher
// 4 接收rtmp数据 转为flv数据
// 5 发送flv数据
func FlvPlayer(c net.Conn) {
	len, err := ReadUint8(c)
	if err != nil {
		log.Println(err)
		return
	}
	//log.Println(len)

	data, err := ReadByte(c, uint32(len))
	if err != nil {
		log.Println(err)
		return
	}

	var fpi FlvPlayInfo
	err = json.Unmarshal(data, &fpi)
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("%#v", fpi)

	s := NewStream(c)
	s.StreamType = "flvPlayer"
	s.AmfInfo.App = fpi.App
	s.AmfInfo.StreamName = fpi.Stream
	s.RemoteAddr = fpi.Client
	s.IsPublisher = false

	s.log.Printf("---> the stream is publisher %t", s.IsPublisher)
	StreamLogRename(s, "flv")

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

// 3 + 1 + 1 + 4 + 4 = 13字节
type FlvHead struct {
	Signature0    uint8  // 8bit, F 0x46
	Signature1    uint8  // 8bit, L 0x4c
	Signature2    uint8  // 8bit, V 0x56
	Version       uint8  // 8bit, 0x01
	FlagReserved0 uint8  // 5bit, 00000
	FlagAudio     uint8  // 1bit, 1
	FlagReserved1 uint8  // 1bit, 0
	FlagVideo     uint8  // 1bit, 1
	Offset        uint32 // 32bit, 0x09
	TagSize       uint32 // 32bit, 0x0
}

// 11 + 4 = 15字节
type FlvTag struct {
	TagType    uint8  // 8bit
	DataSize   uint32 // 24bit
	Timestamp  uint32 // 24bit
	TimeExtend uint8  // 8bit
	StreamId   uint32 // 24bit
	Data       []byte
	TagSize    uint32 // 32bit
}

type FlvData struct {
	Head FlvHead
	Tags []FlvTag
}

func GopCacheSendFlv(s *Stream, gop *GopCache) error {
	var h FlvHead
	h.Signature0 = 0x46
	h.Signature1 = 0x4c
	h.Signature2 = 0x56
	h.Version = 0x01
	h.FlagReserved0 = 0x0
	h.FlagAudio = 0x1
	h.FlagReserved1 = 0x0
	h.FlagVideo = 0x1
	h.Offset = 0x9
	h.TagSize = 0x0
	s.log.Printf("%#v", h)

	FlvSendHead(s, h)
	FlvSendMetaData(s, gop.MetaData)
	FlvSendVideoHead(s, gop.VideoHeader)
	FlvSendAudioHead(s, gop.AudioHeader)
	FlvSendData(s, gop.MediaData)
	return nil
}

func FlvSendHead(s *Stream, h FlvHead) {
	buf := make([]byte, 13)
	buf[0] = h.Signature0
	buf[1] = h.Signature1
	buf[2] = h.Signature2
	buf[3] = h.Version
	buf[4] = (h.FlagReserved0&0x1f)<<3 |
		(h.FlagAudio&0x1)<<2 |
		(h.FlagReserved1&0x1)<<1 |
		(h.FlagVideo & 0x1)
	Uint32ToByte(h.Offset, buf[5:9], BE)
	Uint32ToByte(h.TagSize, buf[9:13], BE)
	s.log.Println(len(buf), buf)

	_, err := s.Conn.Write(buf)
	if err != nil {
		s.log.Println(err)
		return
	}
}

func FlvSendMetaData(s *Stream, c *Chunk) {
	var t FlvTag
	t.TagType = 0x12
	t.DataSize = c.MsgLength
	t.Timestamp = c.Timestamp
	t.TimeExtend = 0x0
	t.StreamId = c.MsgStreamId
	t.Data = nil
	t.TagSize = 11 + t.DataSize
	s.log.Printf("%#v", t)

	// 11 + DataSize + 4
	size := 11 + t.DataSize + 4
	buf := make([]byte, size)
	// 11byte
	buf[0] = t.TagType
	Uint24ToByte(t.DataSize, buf[1:4], BE)
	Uint24ToByte(t.Timestamp, buf[4:7], BE)
	buf[7] = t.TimeExtend
	Uint24ToByte(t.StreamId, buf[8:11], BE)
	// MsgData
	copy(buf[11:11+t.DataSize], c.MsgData)
	// TagSize
	Uint32ToByte(t.TagSize, buf[11+t.DataSize:], BE)
	s.log.Println(len(buf), buf)

	// send data
	_, err := s.Conn.Write(buf)
	if err != nil {
		s.log.Println(err)
		return
	}
}

func FlvSendVideoHead(s *Stream, c *Chunk) {
	var t FlvTag
	t.TagType = 0x9
	t.DataSize = c.MsgLength
	t.Timestamp = c.Timestamp
	t.TimeExtend = 0x0
	t.StreamId = c.MsgStreamId
	t.Data = nil
	t.TagSize = 11 + t.DataSize
	s.log.Printf("%#v", t)

	// 11 + DataSize + 4
	size := 11 + t.DataSize + 4
	buf := make([]byte, size)
	// 11byte
	buf[0] = t.TagType
	Uint24ToByte(t.DataSize, buf[1:4], BE)
	Uint24ToByte(t.Timestamp, buf[4:7], BE)
	buf[7] = t.TimeExtend
	Uint24ToByte(t.StreamId, buf[8:11], BE)
	// MsgData
	copy(buf[11:11+t.DataSize], c.MsgData)
	// TagSize
	Uint32ToByte(t.TagSize, buf[11+t.DataSize:], BE)
	s.log.Println(len(buf), buf)

	// send data
	_, err := s.Conn.Write(buf)
	if err != nil {
		s.log.Println(err)
		return
	}
}

func FlvSendAudioHead(s *Stream, c *Chunk) {
	var t FlvTag
	t.TagType = 0x8
	t.DataSize = c.MsgLength
	t.Timestamp = c.Timestamp
	t.TimeExtend = 0x0
	t.StreamId = c.MsgStreamId
	t.Data = nil
	t.TagSize = 11 + t.DataSize
	s.log.Printf("%#v", t)

	// 11 + DataSize + 4
	size := 11 + t.DataSize + 4
	buf := make([]byte, size)
	// 11byte
	buf[0] = t.TagType
	Uint24ToByte(t.DataSize, buf[1:4], BE)
	Uint24ToByte(t.Timestamp, buf[4:7], BE)
	buf[7] = t.TimeExtend
	Uint24ToByte(t.StreamId, buf[8:11], BE)
	// MsgData
	copy(buf[11:11+t.DataSize], c.MsgData)
	// TagSize
	Uint32ToByte(t.TagSize, buf[11+t.DataSize:], BE)
	s.log.Println(len(buf), buf)

	// send data
	_, err := s.Conn.Write(buf)
	if err != nil {
		s.log.Println(err)
		return
	}
}

func FlvSendData(s *Stream, md *list.List) {
	s.log.Println("@@@ send MediaData")
	s.log.Println("@@@ GopCache.MediaData len", md.Len())

	var t FlvTag
	i := 0
	for e := md.Front(); e != nil; e = e.Next() {
		c := (e.Value).(*Chunk)
		t.TagType = uint8(c.MsgTypeId)
		t.DataSize = c.MsgLength
		t.Timestamp = c.Timestamp
		t.TimeExtend = 0x0
		t.StreamId = c.MsgStreamId
		t.Data = nil
		t.TagSize = 11 + t.DataSize
		s.log.Printf("%#v", t)

		// 11 + DataSize + 4
		size := 11 + t.DataSize + 4
		buf := make([]byte, size)
		// 11byte
		buf[0] = t.TagType
		Uint24ToByte(t.DataSize, buf[1:4], BE)
		Uint24ToByte(t.Timestamp, buf[4:7], BE)
		buf[7] = t.TimeExtend
		Uint24ToByte(t.StreamId, buf[8:11], BE)
		// MsgData
		copy(buf[11:11+t.DataSize], c.MsgData)
		// TagSize
		Uint32ToByte(t.TagSize, buf[11+t.DataSize:], BE)
		s.log.Println(len(buf))
		//log.Println(len(buf), buf)

		// send data
		_, err := s.Conn.Write(buf)
		if err != nil {
			s.log.Println(err)
			s.log.Println("@@@ GopCacheSend() error")
			return
		}
		s.log.Println("@@@ GopCacheSend", i, c.DataType, c.MsgLength, c.Timestamp)
		i++
	}
	s.log.Println("@@@ GopCacheSend() ok")
}

func MessageSendFlv(s *Stream, c *Chunk) error {
	var t FlvTag
	t.TagType = uint8(c.MsgTypeId)
	t.DataSize = c.MsgLength
	t.Timestamp = c.Timestamp
	t.TimeExtend = 0x0
	t.StreamId = c.MsgStreamId
	t.Data = nil
	t.TagSize = 11 + t.DataSize
	s.log.Printf("%#v", t)

	// 11 + DataSize + 4
	size := 11 + t.DataSize + 4
	buf := make([]byte, size)
	// 11byte
	buf[0] = t.TagType
	Uint24ToByte(t.DataSize, buf[1:4], BE)
	Uint24ToByte(t.Timestamp, buf[4:7], BE)
	buf[7] = t.TimeExtend
	Uint24ToByte(t.StreamId, buf[8:11], BE)
	// MsgData
	copy(buf[11:11+t.DataSize], c.MsgData)
	// TagSize
	Uint32ToByte(t.TagSize, buf[11+t.DataSize:], BE)
	s.log.Println(len(buf))
	//log.Println(len(buf), buf)

	// send data
	_, err := s.Conn.Write(buf)
	if err != nil {
		s.log.Println(err)
		return err
	}
	return nil
}
