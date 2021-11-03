package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"livego/protocol/amf"
	"log"
	"net"
	"net/http"
	"os/exec"
	"reflect"
	"sync"
	"unsafe"
)

type Statistics struct {
}

type HlsProducer struct {
}

type MediaInfo struct {
	soundFormat     uint8
	soundRate       uint8
	soundSize       uint8
	soundType       uint8
	aacPacketType   uint8
	frameType       uint8
	codecID         uint8
	avcPacketType   uint8
	compositionTime int32
}

type MStreams struct {
	Streams map[string]interface{}
	sync.RWMutex
}

const (
	amfConnect       = "connect"
	amfFcpublish     = "FCPublish"
	amfReleaseStream = "releaseStream"
	amfCreateStream  = "createStream"
	amfPublish       = "publish"
	amfFCUnpublish   = "FCUnpublish"
	amfDeleteStream  = "deleteStream"
	amfPlay          = "play"
)

const (
	AMF0_NUMBER_MARKER         = 0x00
	AMF0_BOOLEAN_MARKER        = 0x01
	AMF0_STRING_MARKER         = 0x02
	AMF0_OBJECT_MARKER         = 0x03
	AMF0_MOVIECLIP_MARKER      = 0x04
	AMF0_NULL_MARKER           = 0x05
	AMF0_UNDEFINED_MARKER      = 0x06
	AMF0_REFERENCE_MARKER      = 0x07
	AMF0_ECMA_ARRAY_MARKER     = 0x08
	AMF0_OBJECT_END_MARKER     = 0x09
	AMF0_STRICT_ARRAY_MARKER   = 0x0a
	AMF0_DATE_MARKER           = 0x0b
	AMF0_LONG_STRING_MARKER    = 0x0c
	AMF0_UNSUPPORTED_MARKER    = 0x0d
	AMF0_RECORDSET_MARKER      = 0x0e
	AMF0_XML_DOCUMENT_MARKER   = 0x0f
	AMF0_TYPED_OBJECT_MARKER   = 0x10
	AMF0_ACMPLUS_OBJECT_MARKER = 0x11
)

type AmfInfo struct {
	App            string `amf:"app" json:"app"`
	Type           string
	FlashVer       string `amf:"flashVer" json:"flashVer"`
	SwfUrl         string `amf:"swfUrl" json:"swfUrl"`
	TcUrl          string `amf:"tcUrl" json:"tcUrl"`
	Fpad           bool   `amf:"fpad" json:"fpad"`
	AudioCodecs    int    `amf:"audioCodecs" json:"audioCodecs"`
	VideoCodecs    int    `amf:"videoCodecs" json:"videoCodecs"`
	VideoFunction  int    `amf:"videoFunction" json:"videoFunction"`
	PageUrl        string `amf:"pageUrl" json:"pageUrl"`
	ObjectEncoding int    `amf:"objectEncoding" json:"objectEncoding"`
	transactionID  int
	PubName        string
	PubType        string
}

func AmfDisFormat(r io.Reader) (i []interface{}, err error) {
	var v interface{}
	for {
		v, err = AmfDecode(r)
		if err != nil {
			if err != io.EOF {
				log.Println(err)
			}
			break
		}
		i = append(i, v)
	}
	return i, err
}

func AmfDecode(r io.Reader) (interface{}, error) {
	AmfType, err := ReadByteToUint32BE(r, 1)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return nil, err
	}
	log.Println("AmfType: ", AmfType)

	switch AmfType {
	case AMF0_NUMBER_MARKER:
		return Amf0DecodeNumber(r)
	case AMF0_BOOLEAN_MARKER:
		return Amf0DecodeBoolean(r)
	case AMF0_STRING_MARKER:
		return Amf0DecodeString(r)
	case AMF0_OBJECT_MARKER:
		return Amf0DecodeObject(r)
	case AMF0_NULL_MARKER:
		return Amf0DecodeNull(r)
	case AMF0_ECMA_ARRAY_MARKER:
		return Amf0DecodeEcmaArray(r)
	}
	err = fmt.Errorf("Invalid AMF0 Type, %d", AmfType)
	log.Println(err)
	return nil, err
}

func Amf0DecodeEcmaArray(r io.Reader) (Object, error) {
	len, err := ReadByteToUint32BE(r, 4)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println("amf0 array len", len)

	o, err := Amf0DecodeObject(r)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return o, nil
}

func Amf0DecodeNull(r io.Reader) (result interface{}, err error) {
	return
}

type Object map[string]interface{}

func Amf0DecodeObject(r io.Reader) (Object, error) {
	ret := make(Object)
	for {
		len, err := ReadByteToUint32BE(r, 2)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		key := ReadByteToString(r, len)
		if key == "" {
			ReadByteToUint32BE(r, 1)
			break
		}

		value, err := AmfDecode(r)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		ret[key] = value
	}
	//log.Printf("%#v\n", ret)
	return ret, nil
}

func Amf0DecodeString(r io.Reader) (ret string, err error) {
	len, err := ReadByteToUint32BE(r, 2)
	if err != nil {
		log.Println(err)
		return "", err
	}

	ret = ReadByteToString(r, uint32(len))
	return ret, nil
}

func Amf0DecodeBoolean(r io.Reader) (ret bool, err error) {
	var b byte
	err = binary.Read(r, binary.BigEndian, &b)
	if err != nil {
		log.Println(err)
		return false, err
	}
	if b == 0x00 {
		return false, nil
	}
	return true, nil
}

func Amf0DecodeNumber(r io.Reader) (ret float64, err error) {
	err = binary.Read(r, binary.BigEndian, &ret)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return
}

func (s *MStreams) Add(key string, value interface{}) {
	s.Lock()
	s.Streams[key] = value
	s.Unlock()
}

func (s *MStreams) Del(key string) {
	s.Lock()
	delete(s.Streams, key)
	s.Unlock()
}

func (s *MStreams) Get(key string) (interface{}, bool) {
	s.RLock()
	val, ok := s.Streams[key]
	s.RUnlock()
	return val, ok
}

func TestMStreams() {
	s := &MStreams{Streams: make(map[string]interface{})}
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	s.Add("cctv1", "111")
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	s.Add("cctv2", 222)
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	s.Add("cctv2", 2222)
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	v, ok := s.Get("cctv2")
	log.Printf("%v, %#v\n", ok, v)
	s.Del("cctv2")
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
}

func (s *Stream) RtmpHandshakeclient() error {
	log.Println("RtmpHandshakeclient()")
	return nil
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

func Uint32ToByteLE(b []byte, v uint32, len int) {
	for i := 0; i < len; i++ {
		b[i] = byte(v >> uint32(i*8))
	}
}

func WriteUint32ToByteBE(w io.Writer, v uint32, len int) error {
	b := make([]byte, len)
	Uint32ToByteBE(b, v, len)
	if _, err := w.Write(b); err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func WriteUint32ToByteLE(w io.Writer, v uint32, len int) error {
	b := make([]byte, len)
	Uint32ToByteLE(b, v, len)
	if _, err := w.Write(b); err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func ByteToUint32LE(b []byte) uint32 {
	ret := uint32(0)
	for i := 0; i < len(b); i++ {
		ret += uint32(b[i]) << uint32(i*8)
	}
	return ret
}

func ReadByteToUint32LE(r io.Reader, n, tag int) uint32 {
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		log.Println(err)
		return 0
	}
	return ByteToUint32LE(b)
}

func (s *Stream) RtmpTransmitStart() (err error) {
	s.TransmitStart = true

	var p Packet
	for {
		if !s.TransmitStart {
			s.Conn.Close()
			return
		}

		s.RtmpRecvData(&p)

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
	return nil
}

func (s *Stream) RtmpCacheGOP(p *Packet) {
	if p.IsMetadata {
		s.Cache.MetaFull = true
		s.Cache.MetaPacket = p
		return
	}
	if p.IsAudio {
		s.Cache.AudioFull = true
		s.Cache.AudioPacket = p
		return
	}
	if p.IsVideo {
		s.Cache.VideoFull = true
		s.Cache.VideoPacket = p
		return
	}
}

func (s *Stream) RtmpSendGOP() (err error) {
	return
}

const (
	Sound_mp3     = 2
	Sound_aac     = 10
	Sound_5500hz  = 0
	Sound_11000hz = 1
	Sound_22000hz = 2
	Sound_44000hz = 3
	Sound_8bit    = 0
	Sound_16bit   = 1
	Frame_key     = 1
	Frame_inter   = 2
)

func (s *Stream) ParseVideoTagHeader(p *Packet) (n int, err error) {
	tag := p.Data[0]
	s.MediaInfo.frameType = tag >> 4
	s.MediaInfo.codecID = tag & 0xf
	n++
	if s.MediaInfo.frameType == Frame_key || s.MediaInfo.frameType == Frame_inter {
		s.MediaInfo.avcPacketType = p.Data[1]
		for i := 2; i < 5; i++ {
			s.MediaInfo.compositionTime =
				s.MediaInfo.compositionTime<<8 + int32(p.Data[i])
		}
		n += 4
	}
	log.Printf("%#v\n", s.MediaInfo)
	return
}

func (s *Stream) ParseAudioTagHeader(p *Packet) (n int, err error) {
	tag := p.Data[0]
	s.MediaInfo.soundFormat = tag >> 4
	s.MediaInfo.soundRate = (tag >> 2) & 0x3
	s.MediaInfo.soundSize = (tag >> 1) & 0x1
	s.MediaInfo.soundType = tag & 0x1
	n++
	if s.MediaInfo.soundFormat == Sound_aac {
		s.MediaInfo.aacPacketType = p.Data[1]
	}
	log.Printf("%#v\n", s.MediaInfo)
	return
}

func (s *Stream) SendAckMessage(len uint32) {
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
}

func (s *Stream) HandleMessage(msg *Chunk) (err error) {
	// rtmp 消息类型: 视频消息 音频消息 数据消息 共享对象消息 控制消息 命令消息
	// 控制消息 分为: 块流控制消息 和 用户控制消息
	switch msg.TypeID {
	case MsgTypeIdSetChunkSize:
		s.remoteChunkSize = binary.BigEndian.Uint32(msg.Data)
		log.Println("new remoteChunkSize", s.remoteChunkSize)
	case MsgTypeIdUserControlMessages:
		log.Println("MsgTypeIdUserControlMessages")
	case MsgTypeIdWindowAckSize:
		s.remoteWindowAckSize = binary.BigEndian.Uint32(msg.Data)
		log.Println("new remoteWindowAckSize", s.remoteWindowAckSize)
	case MsgTypeIdCmdAmf0Data, MsgTypeIdCmdAmf0Share, MsgTypeIdCmdAmf0Code:
		if err = s.HandleCommandMessage(msg); err != nil {
			log.Println(err)
			return
		}
	default:
		err = fmt.Errorf("Invalid Message Type Id, %d", msg.TypeID)
		log.Println(err)
		return err
	}
	return
}

func ReadByteToString(r io.Reader, n uint32) string {
	if n == 0 {
		return ""
	}

	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		log.Println(err)
		return ""
	}
	return string(b)
}

func (s *Stream) HandleCommandMessage(msg *Chunk) error {
	r := bytes.NewReader(msg.Data)
	vs, err := AmfDisFormat(r)
	if err != nil && err != io.EOF {
		log.Println(err)
		return err
	}
	log.Printf("client rtmp command message %#v\n", vs)
	switch vs[0].(string) {
	case amfConnect:
		if err = s.AmfConnect(vs[1:]); err != nil {
			return err
		}
		if err = s.AmfConnectResp(msg); err != nil {
			return err
		}
	case amfReleaseStream:
		return nil
	case amfFcpublish:
		return nil
	case amfCreateStream:
		if err = s.AmfCreateStream(vs[1:]); err != nil {
			return err
		}
		if err = s.AmfCreateStreamResp(msg); err != nil {
			return err
		}
	case amfPublish:
		if err = s.AmfPublishOrPlay(vs[1:]); err != nil {
			return err
		}
		if err = s.AmfPublishResp(msg); err != nil {
			return err
		}
		s.HandleMessageDone = true
		s.isPublisher = true
	case amfPlay:
		/*
			if err = s.AmfPublishOrPlay(vs[1:]); err != nil {
				return err
			}
			if err = s.AmfPlayResp(msg); err != nil {
				return err
			}
			s.HandleMessageDone = true
			s.isPublisher = false
		*/
	default:
		err = fmt.Errorf("Invalid amf command", vs[0].(string))
		return err
	}
	return nil
}

func (s *Stream) AmfPublishOrPlay(vs []interface{}) error {
	for k, v := range vs {
		switch v.(type) {
		case string:
			if k == 2 {
				s.AmfInfo.PubName = v.(string)
			} else if k == 3 {
				s.AmfInfo.PubType = v.(string)
			}
		case float64:
			s.AmfInfo.transactionID = int(v.(float64))
		case amf.Object:
		}
	}
	return nil
}

func (s *Stream) AmfPublishResp(msg *Chunk) error {
	event := make(Object)
	event["level"] = "status"
	event["code"] = "NetStream.Publish.Start"
	event["description"] = "Start publising."

	amf, _ := AmfFormat("onStatus", 0, nil, event)
	log.Println(amf)
	c := CreateMessage0(msg.Csid, msg.StreamID,
		MsgTypeIdCmdAmf0Code, uint32(len(amf)), amf)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	return nil
}

func (s *Stream) AmfCreateStream(vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case float64:
			s.AmfInfo.transactionID = int(v.(float64))
		}
	}
	return nil
}

func (s *Stream) AmfCreateStreamResp(msg *Chunk) error {
	amf, _ := AmfFormat("_result", s.AmfInfo.transactionID, nil, msg.StreamID)
	log.Println(amf)
	c := CreateMessage0(msg.Csid, msg.StreamID,
		MsgTypeIdCmdAmf0Code, uint32(len(amf)), amf)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	return nil
}

func (s *Stream) AmfConnectResp(msg *Chunk) error {
	c := CreateMessage(MsgTypeIdWindowAckSize, 4, 2500000)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	c = CreateMessage(MsgTypeIdSetPeerBandwidth, 5, 2500000)
	c.Data[4] = 2 // ???
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	c = CreateMessage(MsgTypeIdSetChunkSize, 4, uint32(1024))
	s.chunkSize = ByteToUint32BE(c.Data)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)

	resp := make(Object)
	resp["fmsVer"] = "FMS/3,0,1,123"
	resp["capabilities"] = 31

	event := make(Object)
	event["level"] = "status"
	event["code"] = "NetConnection.Connect.Success"
	event["description"] = "Connection succeeded."
	event["objectEncoding"] = s.AmfInfo.ObjectEncoding
	log.Println(resp, event)
	amf, _ := AmfFormat("_result", s.AmfInfo.transactionID, resp, event)
	log.Println(amf)
	c = CreateMessage0(msg.Csid, msg.StreamID,
		MsgTypeIdCmdAmf0Code, uint32(len(amf)), amf)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	return nil
}

func Amf0EncodeNull(buf io.Writer) (n int, err error) {
	b := []byte{AMF0_NULL_MARKER}
	buf.Write(b)
	n += 1
	return
}

func Amf0EncodeString(buf io.Writer, v string, haveAmfType bool) (n int, err error) {
	if haveAmfType {
		b := []byte{AMF0_STRING_MARKER}
		buf.Write(b)
		n += 1
	}

	length := uint32(len(v))
	WriteUint32ToByteBE(buf, length, 2)
	n += 2

	m, err := buf.Write([]byte(v))
	if err != nil {
		log.Println(err)
		return n, err
	}
	n += m
	return
}

func Amf0EncodeBool(buf io.Writer, v bool) (n int, err error) {
	b := []byte{AMF0_BOOLEAN_MARKER}
	buf.Write(b)
	n += 1

	if v {
		b[0] = 0x01
	} else {
		b[0] = 0x00
	}

	m, err := buf.Write(b)
	if err != nil {
		log.Println(err)
		return n, err
	}
	n += m
	return
}

func Amf0EncodeNumber(buf io.Writer, v float64) (n int, err error) {
	b := []byte{AMF0_NUMBER_MARKER}
	buf.Write(b)
	n += 1

	err = binary.Write(buf, binary.BigEndian, &v)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	n += 8
	return
}

func Amf0EncodeObject(buf io.Writer, o Object) (n int, err error) {
	b := []byte{AMF0_OBJECT_MARKER}
	buf.Write(b)
	n += 1

	m := 0
	for k, v := range o {
		m, err = Amf0EncodeString(buf, k, false)
		if err != nil {
			log.Println(err)
			return 0, err
		}
		n += m

		m, err = AmfEncode(buf, v)
		if err != nil {
			log.Println(err)
			return 0, err
		}
		n += m
	}

	m, err = Amf0EncodeString(buf, "", false)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	n += m

	b[0] = AMF0_OBJECT_END_MARKER
	buf.Write(b)
	n += 1
	return
}

func AmfEncode(buf io.Writer, v interface{}) (int, error) {
	if v == nil {
		return Amf0EncodeNull(buf)
	}

	val := reflect.ValueOf(v)
	log.Println(v, val.Kind())
	switch val.Kind() {
	case reflect.String:
		return Amf0EncodeString(buf, val.String(), true)
	case reflect.Bool:
		return Amf0EncodeBool(buf, val.Bool())
	case reflect.Int:
		return Amf0EncodeNumber(buf, float64(val.Int()))
	case reflect.Uint32:
		return Amf0EncodeNumber(buf, float64(val.Uint()))
	case reflect.Float32, reflect.Float64:
		return Amf0EncodeNumber(buf, float64(val.Float()))
	case reflect.Map:
		return Amf0EncodeObject(buf, v.(Object))
	}
	err := fmt.Errorf("Invalid Kind() type %s", val.Kind())
	log.Println(err)
	return 0, err
}

func AmfFormat(args ...interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	for _, v := range args {
		if _, err := AmfEncode(buf, v); err != nil {
			log.Println(err)
			return nil, err
		}
	}
	amf := buf.Bytes()
	return amf, nil
}

func CreateMessage0(csid, streamId, typeid, length uint32, data []byte) Chunk {
	c := Chunk{
		Fmt:      0,
		Csid:     csid,
		Length:   length,
		TypeID:   typeid,
		StreamID: streamId,
		Data:     data,
	}
	return c
}

func CreateMessage(typeid, length, data uint32) Chunk {
	c := Chunk{
		Fmt:      0,
		Csid:     2,
		Length:   length,
		TypeID:   typeid,
		StreamID: 0,
		Data:     make([]byte, length),
	}
	if length > 4 {
		length = 4
	}
	Uint32ToByteBE(c.Data[:length], data, int(length))
	return c
}

func (s *Stream) AmfConnect(vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case float64:
			id := int(v.(float64))
			s.AmfInfo.transactionID = id
		case string:
		case Object:
			o := v.(Object)
			if i, ok := o["app"]; ok {
				s.AmfInfo.App = i.(string)
			}
			if i, ok := o["type"]; ok {
				s.AmfInfo.Type = i.(string)
			}
			if i, ok := o["flashVer"]; ok {
				s.AmfInfo.FlashVer = i.(string)
			}
			if i, ok := o["tcUrl"]; ok {
				s.AmfInfo.TcUrl = i.(string)
			}
			if i, ok := o["objectEncoding"]; ok {
				s.AmfInfo.ObjectEncoding = int(i.(float64))
			}
		}
	}
	log.Printf("%#v\n", s.AmfInfo)
	return nil
}

func TestSize() {
	var f32 float32
	var f64 float64
	var b bool
	log.Println("float32 size", unsafe.Sizeof(f32))
	log.Println("float64 size", unsafe.Sizeof(f64))
	log.Println("bool size", unsafe.Sizeof(b))
}

func RouteDefaultIface() string {
	cmd := exec.Command("/bin/sh", "-c", `route | grep default | awk '{print $8}'`)
	b, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(err)
		return ""

	}
	return string(b[:len(b)-1])

}

func IfaceIP(name string) string {
	var ip string
	iface, err := net.InterfaceByName(name)
	if err != nil {
		log.Println(err)
		return ""

	}
	addrs, _ := iface.Addrs()
	for _, addr := range addrs {
		ipnet, _ := addr.(*net.IPNet)
		if ipnet.IP.To4() != nil {
			//log.Println(ipnet.IP.String())
			ip = ipnet.IP.String()
			break
		}
	}
	return ip
}

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	live = Live{Config{"1935", "6666", "7777", "8888"},
		MStreams{Streams: make(map[string]interface{})}}
	log.Println(live)
}

func (c *Chunk) ChunkDisAssmbleHeader(w io.Writer) error {
	h := c.Fmt << 6
	switch {
	case c.Csid < 64:
		h |= c.Csid
		WriteUint32ToByteBE(w, h, 1)
	case c.Csid-64 < 256:
		h |= 0
		WriteUint32ToByteBE(w, h, 1)
		WriteUint32ToByteBE(w, c.Csid-64, 1) // xxx LE()
	case c.Csid-64 < 65535:
		h |= 0
		WriteUint32ToByteBE(w, h, 1)
		WriteUint32ToByteBE(w, c.Csid-64, 2) // xxx LE()
	}

	if c.Fmt == 3 {
		goto END
	}
	if c.Timestamp > 0xffffff {
		WriteUint32ToByteBE(w, 0xffffff, 3)
	} else {
		WriteUint32ToByteBE(w, c.Timestamp, 3)
	}

	if c.Fmt == 2 {
		goto END
	}
	WriteUint32ToByteBE(w, c.Length, 3)
	WriteUint32ToByteBE(w, c.TypeID, 1)

	if c.Fmt == 1 {
		goto END
	}
	WriteUint32ToByteBE(w, c.StreamID, 4)
END:
	if c.Timestamp > 0xffffff {
		WriteUint32ToByteBE(w, c.Timestamp, 4)
	}
	return nil
}

func (c *Chunk) ChunkDisAssmble(w io.Writer, ChunkSize uint32) error {
	SendLen := uint32(0)
	s, e, d := uint32(0), uint32(0), uint32(0)
	n := c.Length / ChunkSize
	log.Println(c.Length, ChunkSize, n)
	for i := uint32(0); i <= n; i++ {
		if SendLen == c.Length {
			log.Println("message send over")
			break
		}

		c.Fmt = uint32(0)
		if i != 0 {
			c.Fmt = uint32(3)
		}

		c.ChunkDisAssmbleHeader(w)

		s = i * ChunkSize
		d = uint32(len(c.Data)) - s
		if d > ChunkSize {
			e = s + ChunkSize
			SendLen += ChunkSize
		} else {
			e = s + d
			SendLen += d
		}
		buf := c.Data[s:e]
		if _, err := w.Write(buf); err != nil {
			log.Println(err)
			return err
		}
	}
	return nil
}

// xxx
func (c *Chunk) ChunkAssmble(r io.Reader, chunkSize uint32) error {
	// fmt: 控制Message Header的类型, 0表示11字节, 1表示7字节, 2表示3字节, 3表示0字节
	switch c.Fmt {
	case 0:
		c.Timestamp, _ = ReadByteToUint32BE(r, 3)
		c.Length, _ = ReadByteToUint32BE(r, 3)
		c.TypeID, _ = ReadByteToUint32BE(r, 1)
		c.StreamID = ReadByteToUint32LE(r, 4, 1) // xxx
		if c.Timestamp == 0xffffff {
			c.Timestamp, _ = ReadByteToUint32BE(r, 4)
			c.TimeExted = true
		} else {
			c.TimeExted = false
		}

		c.Data = make([]byte, c.Length)
		c.Index = 0
		c.Remain = c.Length
		c.Full = false
	case 1:
		c.TimeDelta, _ = ReadByteToUint32BE(r, 3)
		c.Length, _ = ReadByteToUint32BE(r, 3)
		c.TypeID, _ = ReadByteToUint32BE(r, 1)
		if c.TimeDelta == 0xffffff {
			c.TimeDelta, _ = ReadByteToUint32BE(r, 4)
			c.TimeExted = true
		} else {
			c.TimeExted = false
		}
		c.Timestamp += c.TimeDelta

		c.Data = make([]byte, c.Length)
		c.Index = 0
		c.Remain = c.Length
		c.Full = false
	case 2:
		c.TimeDelta, _ = ReadByteToUint32BE(r, 3)
		if c.TimeDelta == 0xffffff {
			c.TimeDelta, _ = ReadByteToUint32BE(r, 4)
			c.TimeExted = true
		} else {
			c.TimeExted = false
		}
		c.Timestamp += c.TimeDelta

		c.Data = make([]byte, c.Length)
		c.Index = 0
		c.Remain = c.Length
		c.Full = false
	case 3:
	default:
		return fmt.Errorf("Invalid fmt=%d", c.Fmt)
	}

	size := uint32(c.Remain)
	if size > chunkSize {
		size = chunkSize
	}

	buf := c.Data[c.Index : c.Index+size]
	if _, err := r.Read(buf); err != nil {
		log.Println(err)
		return err
	}
	c.Index += size
	c.Remain -= size
	if c.Remain == 0 {
		c.Full = true
		log.Println("Message TypeID:", c.TypeID)
		log.Printf("%#v\n", c)
	}

	return nil
}

const (
	amfConnect       = "connect"
	amfFcpublish     = "FCPublish"
	amfReleaseStream = "releaseStream"
	amfCreateStream  = "createStream"
	amfPublish       = "publish"
	amfFCUnpublish   = "FCUnpublish"
	amfDeleteStream  = "deleteStream"
	amfPlay          = "play"
)

type AmfInfo struct {
	App            string `amf:"app" json:"app"`
	Type           string
	FlashVer       string `amf:"flashVer" json:"flashVer"`
	SwfUrl         string `amf:"swfUrl" json:"swfUrl"`
	TcUrl          string `amf:"tcUrl" json:"tcUrl"`
	Fpad           bool   `amf:"fpad" json:"fpad"`
	AudioCodecs    int    `amf:"audioCodecs" json:"audioCodecs"`
	VideoCodecs    int    `amf:"videoCodecs" json:"videoCodecs"`
	VideoFunction  int    `amf:"videoFunction" json:"videoFunction"`
	PageUrl        string `amf:"pageUrl" json:"pageUrl"`
	ObjectEncoding int    `amf:"objectEncoding" json:"objectEncoding"`
	transactionID  int
	PubName        string
	PubType        string
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

type Statistics struct {
}

type RtmpPublisher struct {
}

type RtmpPlayer struct {
	Start bool
}

type HlsProducer struct {
}

type MediaInfo struct {
	soundFormat     uint8
	soundRate       uint8
	soundSize       uint8
	soundType       uint8
	aacPacketType   uint8
	frameType       uint8
	codecID         uint8
	avcPacketType   uint8
	compositionTime int32
}

type Config struct {
	RtmpPort string
	FlvPort  string
	HlsPort  string
	OptPort  string
}

type MStreams struct {
	Streams map[string]interface{}
	sync.RWMutex
}

func (s *MStreams) Add(key string, value interface{}) {
	s.Lock()
	s.Streams[key] = value
	s.Unlock()
}

func (s *MStreams) Del(key string) {
	s.Lock()
	delete(s.Streams, key)
	s.Unlock()
}

func (s *MStreams) Get(key string) (interface{}, bool) {
	s.RLock()
	val, ok := s.Streams[key]
	s.RUnlock()
	return val, ok
}

func TestMStreams() {
	s := &MStreams{Streams: make(map[string]interface{})}
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	s.Add("cctv1", "111")
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	s.Add("cctv2", 222)
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	s.Add("cctv2", 2222)
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
	v, ok := s.Get("cctv2")
	log.Printf("%v, %#v\n", ok, v)
	s.Del("cctv2")
	log.Printf("%d, %#v\n", len(s.Streams), s.Streams)
}

func (s *Stream) RtmpHandshakeclient() error {
	log.Println("RtmpHandshakeclient()")
	return nil
}

func (s *Stream) RtmpHandshakeServer() (err error) {
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

	if C0[0] != 3 {
		err = fmt.Errorf("invalid client rtmp version %d", C0[0])
		log.Println(err)
		return
	}
	S0[0] = 3

	cVersion := ByteToUint32BE(C1[4:8])
	log.Println("cVersion:", cVersion)
	if cVersion != 0 {
		log.Println("rtmp complex handshake")
		err = RtmpHandshakeServerComplex(C1, S1, S2)
		if err != nil {
			log.Println(err)
			return
		}
	} else {
		log.Println("rtmp simple handshake")
		copy(S1, C2)
		copy(S2, C1)
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

/*
	rtmp handshake
	rtmp handle message
	if publish {
		go recive data
		if hls {
			register hls play
		}
		if transfer {
			register rtmp publish
		}
	} else {
		register rtmp play
	}
*/

func (s *Stream) RtmpCacheGOP(p *Packet) {
	if p.IsMetadata {
		s.Cache.MetaFull = true
		s.Cache.MetaPacket = p
		return
	}
	if p.IsAudio {
		s.Cache.AudioFull = true
		s.Cache.AudioPacket = p
		return
	}
	if p.IsVideo {
		s.Cache.VideoFull = true
		s.Cache.VideoPacket = p
		return
	}
}

func (s *Stream) RtmpSendGOP() (err error) {
	return
}

func (s *Stream) RtmpRecvData(p *Packet) (err error) {
	msg := Chunk{}
	for {
		if err = s.HandleChunk(&msg); err != nil {
			log.Println(err)
			return err
		}
		log.Println("xxx", msg.TypeID)

		if msg.TypeID == MsgTypeIdAudioData ||
			msg.TypeID == MsgTypeIdVideoData ||
			msg.TypeID == MsgTypeIdCmdAmf3Data ||
			msg.TypeID == MsgTypeIdCmdAmf0Data {
			break
		}
	}

	p.IsAudio = msg.TypeID == MsgTypeIdAudioData
	p.IsVideo = msg.TypeID == MsgTypeIdVideoData
	p.IsMetadata = msg.TypeID == MsgTypeIdCmdAmf3Data ||
		msg.TypeID == MsgTypeIdCmdAmf0Data
	p.StreamID = msg.StreamID
	p.Timestamp = msg.Timestamp
	p.Data = msg.Data
	log.Printf("%#v", p)

	if p.IsMetadata {
		vs, err := AmfDisFormat(bytes.NewReader(msg.Data))
		if err != nil && err != io.EOF {
			log.Println(err)
		} else {
			log.Printf("client rtmp command message %#v\n", vs)
		}
	}
	if p.IsAudio {
		s.ParseAudioTagHeader(p)
	}
	if p.IsVideo {
		s.ParseVideoTagHeader(p)
	}
	return nil
}

const (
	Sound_mp3     = 2
	Sound_aac     = 10
	Sound_5500hz  = 0
	Sound_11000hz = 1
	Sound_22000hz = 2
	Sound_44000hz = 3
	Sound_8bit    = 0
	Sound_16bit   = 1
	Frame_key     = 1
	Frame_inter   = 2
)

func (s *Stream) ParseVideoTagHeader(p *Packet) (n int, err error) {
	tag := p.Data[0]
	s.MediaInfo.frameType = tag >> 4
	s.MediaInfo.codecID = tag & 0xf
	n++
	if s.MediaInfo.frameType == Frame_key || s.MediaInfo.frameType == Frame_inter {
		s.MediaInfo.avcPacketType = p.Data[1]
		for i := 2; i < 5; i++ {
			s.MediaInfo.compositionTime =
				s.MediaInfo.compositionTime<<8 + int32(p.Data[i])
		}
		n += 4
	}
	log.Printf("%#v\n", s.MediaInfo)
	return
}

func (s *Stream) ParseAudioTagHeader(p *Packet) (n int, err error) {
	tag := p.Data[0]
	s.MediaInfo.soundFormat = tag >> 4
	s.MediaInfo.soundRate = (tag >> 2) & 0x3
	s.MediaInfo.soundSize = (tag >> 1) & 0x1
	s.MediaInfo.soundType = tag & 0x1
	n++
	if s.MediaInfo.soundFormat == Sound_aac {
		s.MediaInfo.aacPacketType = p.Data[1]
	}
	log.Printf("%#v\n", s.MediaInfo)
	return
}

func (s *Stream) SendAckMessage(len uint32) {
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
}

func (s *Stream) HandleMessage(msg *Chunk) (err error) {
	// rtmp 消息类型: 视频消息 音频消息 数据消息 共享对象消息 控制消息 命令消息
	// 控制消息 分为: 块流控制消息 和 用户控制消息
	switch msg.TypeID {
	case MsgTypeIdSetChunkSize:
		s.remoteChunkSize = binary.BigEndian.Uint32(msg.Data)
		log.Println("new remoteChunkSize", s.remoteChunkSize)
	case MsgTypeIdUserControlMessages:
		log.Println("MsgTypeIdUserControlMessages")
	case MsgTypeIdWindowAckSize:
		s.remoteWindowAckSize = binary.BigEndian.Uint32(msg.Data)
		log.Println("new remoteWindowAckSize", s.remoteWindowAckSize)
	case MsgTypeIdCmdAmf0Data, MsgTypeIdCmdAmf0Share, MsgTypeIdCmdAmf0Code:
		if err = s.HandleCommandMessage(msg); err != nil {
			log.Println(err)
			return
		}
	default:
		err = fmt.Errorf("Invalid Message Type Id, %d", msg.TypeID)
		log.Println(err)
		return err
	}
	return
}

const (
	AMF0_NUMBER_MARKER         = 0x00
	AMF0_BOOLEAN_MARKER        = 0x01
	AMF0_STRING_MARKER         = 0x02
	AMF0_OBJECT_MARKER         = 0x03
	AMF0_MOVIECLIP_MARKER      = 0x04
	AMF0_NULL_MARKER           = 0x05
	AMF0_UNDEFINED_MARKER      = 0x06
	AMF0_REFERENCE_MARKER      = 0x07
	AMF0_ECMA_ARRAY_MARKER     = 0x08
	AMF0_OBJECT_END_MARKER     = 0x09
	AMF0_STRICT_ARRAY_MARKER   = 0x0a
	AMF0_DATE_MARKER           = 0x0b
	AMF0_LONG_STRING_MARKER    = 0x0c
	AMF0_UNSUPPORTED_MARKER    = 0x0d
	AMF0_RECORDSET_MARKER      = 0x0e
	AMF0_XML_DOCUMENT_MARKER   = 0x0f
	AMF0_TYPED_OBJECT_MARKER   = 0x10
	AMF0_ACMPLUS_OBJECT_MARKER = 0x11
)

func ReadByteToString(r io.Reader, n uint32) string {
	if n == 0 {
		return ""
	}

	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		log.Println(err)
		return ""
	}
	return string(b)
}

func AmfDisFormat(r io.Reader) (i []interface{}, err error) {
	var v interface{}
	for {
		v, err = AmfDecode(r)
		if err != nil {
			if err != io.EOF {
				log.Println(err)
			}
			break
		}
		i = append(i, v)
	}
	return i, err
}

func AmfDecode(r io.Reader) (interface{}, error) {
	AmfType, err := ReadByteToUint32BE(r, 1)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return nil, err
	}
	log.Println("AmfType: ", AmfType)

	switch AmfType {
	case AMF0_NUMBER_MARKER:
		return Amf0DecodeNumber(r)
	case AMF0_BOOLEAN_MARKER:
		return Amf0DecodeBoolean(r)
	case AMF0_STRING_MARKER:
		return Amf0DecodeString(r)
	case AMF0_OBJECT_MARKER:
		return Amf0DecodeObject(r)
	case AMF0_NULL_MARKER:
		return Amf0DecodeNull(r)
	case AMF0_ECMA_ARRAY_MARKER:
		return Amf0DecodeEcmaArray(r)
	}
	err = fmt.Errorf("Invalid AMF0 Type, %d", AmfType)
	log.Println(err)
	return nil, err
}

func Amf0DecodeEcmaArray(r io.Reader) (Object, error) {
	len, err := ReadByteToUint32BE(r, 4)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println("amf0 array len", len)

	o, err := Amf0DecodeObject(r)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return o, nil
}

func Amf0DecodeNull(r io.Reader) (result interface{}, err error) {
	return
}

type Object map[string]interface{}

func Amf0DecodeObject(r io.Reader) (Object, error) {
	ret := make(Object)
	for {
		len, err := ReadByteToUint32BE(r, 2)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		key := ReadByteToString(r, len)
		if key == "" {
			ReadByteToUint32BE(r, 1)
			break
		}

		value, err := AmfDecode(r)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		ret[key] = value
	}
	//log.Printf("%#v\n", ret)
	return ret, nil
}

func Amf0DecodeString(r io.Reader) (ret string, err error) {
	len, err := ReadByteToUint32BE(r, 2)
	if err != nil {
		log.Println(err)
		return "", err
	}

	ret = ReadByteToString(r, uint32(len))
	return ret, nil
}

func Amf0DecodeBoolean(r io.Reader) (ret bool, err error) {
	var b byte
	err = binary.Read(r, binary.BigEndian, &b)
	if err != nil {
		log.Println(err)
		return false, err
	}
	if b == 0x00 {
		return false, nil
	}
	return true, nil
}

func Amf0DecodeNumber(r io.Reader) (ret float64, err error) {
	err = binary.Read(r, binary.BigEndian, &ret)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return
}

func (s *Stream) HandleCommandMessage(msg *Chunk) error {
	r := bytes.NewReader(msg.Data)
	vs, err := AmfDisFormat(r)
	if err != nil && err != io.EOF {
		log.Println(err)
		return err
	}
	log.Printf("client rtmp command message %#v\n", vs)
	switch vs[0].(string) {
	case amfConnect:
		if err = s.AmfConnect(vs[1:]); err != nil {
			return err
		}
		if err = s.AmfConnectResp(msg); err != nil {
			return err
		}
	case amfReleaseStream:
		return nil
	case amfFcpublish:
		return nil
	case amfCreateStream:
		if err = s.AmfCreateStream(vs[1:]); err != nil {
			return err
		}
		if err = s.AmfCreateStreamResp(msg); err != nil {
			return err
		}
	case amfPublish:
		if err = s.AmfPublishOrPlay(vs[1:]); err != nil {
			return err
		}
		if err = s.AmfPublishResp(msg); err != nil {
			return err
		}
		s.HandleMessageDone = true
		s.isPublisher = true
	case amfPlay:
		/*
			if err = s.AmfPublishOrPlay(vs[1:]); err != nil {
				return err
			}
			if err = s.AmfPlayResp(msg); err != nil {
				return err
			}
			s.HandleMessageDone = true
			s.isPublisher = false
		*/
	default:
		err = fmt.Errorf("Invalid amf command", vs[0].(string))
		return err
	}
	return nil
}

func (s *Stream) AmfPublishOrPlay(vs []interface{}) error {
	for k, v := range vs {
		switch v.(type) {
		case string:
			if k == 2 {
				s.AmfInfo.PubName = v.(string)
			} else if k == 3 {
				s.AmfInfo.PubType = v.(string)
			}
		case float64:
			s.AmfInfo.transactionID = int(v.(float64))
		case amf.Object:
		}
	}
	return nil
}

func (s *Stream) AmfPublishResp(msg *Chunk) error {
	event := make(Object)
	event["level"] = "status"
	event["code"] = "NetStream.Publish.Start"
	event["description"] = "Start publising."

	amf, _ := AmfFormat("onStatus", 0, nil, event)
	log.Println(amf)
	c := CreateMessage0(msg.Csid, msg.StreamID,
		MsgTypeIdCmdAmf0Code, uint32(len(amf)), amf)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	return nil
}

func (s *Stream) AmfCreateStream(vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case float64:
			s.AmfInfo.transactionID = int(v.(float64))
		}
	}
	return nil
}

func (s *Stream) AmfCreateStreamResp(msg *Chunk) error {
	amf, _ := AmfFormat("_result", s.AmfInfo.transactionID, nil, msg.StreamID)
	log.Println(amf)
	c := CreateMessage0(msg.Csid, msg.StreamID,
		MsgTypeIdCmdAmf0Code, uint32(len(amf)), amf)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	return nil
}

func (s *Stream) AmfConnectResp(msg *Chunk) error {
	c := CreateMessage(MsgTypeIdWindowAckSize, 4, 2500000)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	c = CreateMessage(MsgTypeIdSetPeerBandwidth, 5, 2500000)
	c.Data[4] = 2 // ???
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	c = CreateMessage(MsgTypeIdSetChunkSize, 4, uint32(1024))
	s.chunkSize = ByteToUint32BE(c.Data)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)

	resp := make(Object)
	resp["fmsVer"] = "FMS/3,0,1,123"
	resp["capabilities"] = 31

	event := make(Object)
	event["level"] = "status"
	event["code"] = "NetConnection.Connect.Success"
	event["description"] = "Connection succeeded."
	event["objectEncoding"] = s.AmfInfo.ObjectEncoding
	log.Println(resp, event)
	amf, _ := AmfFormat("_result", s.AmfInfo.transactionID, resp, event)
	log.Println(amf)
	c = CreateMessage0(msg.Csid, msg.StreamID,
		MsgTypeIdCmdAmf0Code, uint32(len(amf)), amf)
	c.ChunkDisAssmble(s.Conn, s.chunkSize)
	return nil
}

func Amf0EncodeNull(buf io.Writer) (n int, err error) {
	b := []byte{AMF0_NULL_MARKER}
	buf.Write(b)
	n += 1
	return
}

func Amf0EncodeString(buf io.Writer, v string, haveAmfType bool) (n int, err error) {
	if haveAmfType {
		b := []byte{AMF0_STRING_MARKER}
		buf.Write(b)
		n += 1
	}

	length := uint32(len(v))
	WriteUint32ToByteBE(buf, length, 2)
	n += 2

	m, err := buf.Write([]byte(v))
	if err != nil {
		log.Println(err)
		return n, err
	}
	n += m
	return
}

func Amf0EncodeBool(buf io.Writer, v bool) (n int, err error) {
	b := []byte{AMF0_BOOLEAN_MARKER}
	buf.Write(b)
	n += 1

	if v {
		b[0] = 0x01
	} else {
		b[0] = 0x00
	}

	m, err := buf.Write(b)
	if err != nil {
		log.Println(err)
		return n, err
	}
	n += m
	return
}

func Amf0EncodeNumber(buf io.Writer, v float64) (n int, err error) {
	b := []byte{AMF0_NUMBER_MARKER}
	buf.Write(b)
	n += 1

	err = binary.Write(buf, binary.BigEndian, &v)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	n += 8
	return
}

func Amf0EncodeObject(buf io.Writer, o Object) (n int, err error) {
	b := []byte{AMF0_OBJECT_MARKER}
	buf.Write(b)
	n += 1

	m := 0
	for k, v := range o {
		m, err = Amf0EncodeString(buf, k, false)
		if err != nil {
			log.Println(err)
			return 0, err
		}
		n += m

		m, err = AmfEncode(buf, v)
		if err != nil {
			log.Println(err)
			return 0, err
		}
		n += m
	}

	m, err = Amf0EncodeString(buf, "", false)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	n += m

	b[0] = AMF0_OBJECT_END_MARKER
	buf.Write(b)
	n += 1
	return
}

func AmfEncode(buf io.Writer, v interface{}) (int, error) {
	if v == nil {
		return Amf0EncodeNull(buf)
	}

	val := reflect.ValueOf(v)
	log.Println(v, val.Kind())
	switch val.Kind() {
	case reflect.String:
		return Amf0EncodeString(buf, val.String(), true)
	case reflect.Bool:
		return Amf0EncodeBool(buf, val.Bool())
	case reflect.Int:
		return Amf0EncodeNumber(buf, float64(val.Int()))
	case reflect.Uint32:
		return Amf0EncodeNumber(buf, float64(val.Uint()))
	case reflect.Float32, reflect.Float64:
		return Amf0EncodeNumber(buf, float64(val.Float()))
	case reflect.Map:
		return Amf0EncodeObject(buf, v.(Object))
	}
	err := fmt.Errorf("Invalid Kind() type %s", val.Kind())
	log.Println(err)
	return 0, err
}

func AmfFormat(args ...interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	for _, v := range args {
		if _, err := AmfEncode(buf, v); err != nil {
			log.Println(err)
			return nil, err
		}
	}
	amf := buf.Bytes()
	return amf, nil
}

func CreateMessage0(csid, streamId, typeid, length uint32, data []byte) Chunk {
	c := Chunk{
		Fmt:      0,
		Csid:     csid,
		Length:   length,
		TypeID:   typeid,
		StreamID: streamId,
		Data:     data,
	}
	return c
}

func CreateMessage(typeid, length, data uint32) Chunk {
	c := Chunk{
		Fmt:      0,
		Csid:     2,
		Length:   length,
		TypeID:   typeid,
		StreamID: 0,
		Data:     make([]byte, length),
	}
	if length > 4 {
		length = 4
	}
	Uint32ToByteBE(c.Data[:length], data, int(length))
	return c
}

func (s *Stream) AmfConnect(vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case float64:
			id := int(v.(float64))
			s.AmfInfo.transactionID = id
		case string:
		case Object:
			o := v.(Object)
			if i, ok := o["app"]; ok {
				s.AmfInfo.App = i.(string)
			}
			if i, ok := o["type"]; ok {
				s.AmfInfo.Type = i.(string)
			}
			if i, ok := o["flashVer"]; ok {
				s.AmfInfo.FlashVer = i.(string)
			}
			if i, ok := o["tcUrl"]; ok {
				s.AmfInfo.TcUrl = i.(string)
			}
			if i, ok := o["objectEncoding"]; ok {
				s.AmfInfo.ObjectEncoding = int(i.(float64))
			}
		}
	}
	log.Printf("%#v\n", s.AmfInfo)
	return nil
}

func TestSize() {
	var f32 float32
	var f64 float64
	var b bool
	log.Println("float32 size", unsafe.Sizeof(f32))
	log.Println("float64 size", unsafe.Sizeof(f64))
	log.Println("bool size", unsafe.Sizeof(b))
}

func RouteDefaultIface() string {
	cmd := exec.Command("/bin/sh", "-c", `route | grep default | awk '{print $8}'`)
	b, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(err)
		return ""

	}
	return string(b[:len(b)-1])

}

func IfaceIP(name string) string {
	var ip string
	iface, err := net.InterfaceByName(name)
	if err != nil {
		log.Println(err)
		return ""

	}
	addrs, _ := iface.Addrs()
	for _, addr := range addrs {
		ipnet, _ := addr.(*net.IPNet)
		if ipnet.IP.To4() != nil {
			//log.Println(ipnet.IP.String())
			ip = ipnet.IP.String()
			break
		}
	}
	return ip
}

func main() {
	//TestSize()
	//TestMStreams()
	go RtmpServer(fmt.Sprintf("%s:%s", IfaceIP("eth1"), "1935"))
	http.HandleFunc("/", HttpServer)
	http.ListenAndServe(":8080", nil)
}

const (
	MsgTypeIdSetChunkSize        = 1  //默认128byte, 最大16777215(0xFFFFFF)
	MsgTypeIdAbortMessage        = 2  //终止消息
	MsgTypeIdAck                 = 3  //块流控制消息, 确认
	MsgTypeIdUserControlMessages = 4  //用户控制消息
	MsgTypeIdWindowAckSize       = 5  //窗口大小
	MsgTypeIdSetPeerBandwidth    = 6  //设置对端带宽
	MsgTypeIdAudioData           = 8  //音频消息
	MsgTypeIdVideoData           = 9  //视频消息
	MsgTypeIdCmdAmf3Data         = 15 //AMF3数据消息
	MsgTypeIdCmdAmf0Data         = 18 //AMF0数据消息
	MsgTypeIdCmdAmf3Share        = 16 //AMF3共享对象消息
	MsgTypeIdCmdAmf0Share        = 19 //AMF0共享对象消息
	MsgTypeIdCmdAmf3Code         = 17 //AMF3命令消息
	MsgTypeIdCmdAmf0Code         = 20 //AMF0命令消息
)

func ReadChunk() {
	n, err := c.Read(buf)
	log.Println(n, err)
	h := buf[0]
	fmt := h >> 6
	csid := h & 0x3f
	log.Println(fmt, csid)
	var cs ChunkStream
	for {
		if err := c.Read(&cs); err != nil {
			return err
		}
	}
	return nil
}

func rtmphandchunk(c net.Conn) error {
	var c ChunkStream
	done := false
	for {
		if err := ReadChunk(); err != nil {
			return err
		}
		switch c.TypeID {
		case 17, 20:
			if err := handleCmdMsg(); err != nil {
				return err
			}
		}
		if down {
			break
		}
	}
	return nil
}

func hsCreate2(p []byte, key []byte) {
	rand.Read(p)
	gap := len(p) - 32
	digest := hsMakeDigest(key, p, gap)
	log.Println("S2中的digest: ", digest)
	copy(p[gap:], digest)
}

func PutU32BE(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

func hsCreate01(p []byte, time uint32, ver uint32, key []byte) {
	p1 := p[1:]
	rand.Read(p1[8:])
	PutU32BE(p1[0:4], time)
	PutU32BE(p1[4:8], ver)
	gap := hsCalcDigestPos(p1, 8)
	digest := hsMakeDigest(key, p1, gap)
	log.Println("S1中的digest: ", digest)
	copy(p1[gap:], digest)
}

func hsParse1(p []byte, fpkeyp []byte, fmskey []byte) (ok bool, digest []byte) {
	var pos int
	if pos = hsFindDigestPos(p, fpkeyp, 772); pos == -1 {
		if pos = hsFindDigestPos(p, fpkeyp, 8); pos == -1 {
			return
		}
	}
	ok = true
	digest = hsMakeDigest(fmskey, p[pos:pos+32], -1)
	return
}

func hsFindDigestPos(p []byte, fpkeyp []byte, base int) int {
	pos := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(fpkeyp, p, pos)
	log.Println("C1中的digest: ", p[pos:pos+32])
	log.Println("C1中计算出的digest: ", digest)
	if bytes.Compare(p[pos:pos+32], digest) != 0 {
		return -1
	}
	return pos
}

func hsCalcDigestPos(p []byte, base int) (pos int) {
	for i := 0; i < 4; i++ {
		pos += int(p[base+i]) // ???
	}
	pos = (pos % 728) + base + 4 // ???
	return
}

func hsMakeDigest(key []byte, src []byte, pos int) (dst []byte) {
	h := hmac.New(sha256.New, key)
	if pos <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:pos])
		h.Write(src[pos+32:])
	}
	return h.Sum(nil)
}
