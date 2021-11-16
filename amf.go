package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"reflect"
)

const (
	Amf0MarkerNumber        = 0x00 // 1byte类型，8byte数据(double类型)
	Amf0MarkerBoolen        = 0x01 // 1byte类型, 1byte数据
	Amf0MarkerString        = 0x02 // 1byte类型，2byte长度，Nbyte数据
	Amf0MarkerObject        = 0x03 // 1byte类型，然后是N个kv键值对，最后00 00 09; kv键值对: key为字符串(不需要类型标识了) 2byte长度 Nbyte数据, value可以是任意amf数据类型 包括object类型
	Amf0MarkerMovieClip     = 0x04
	Amf0MarkerNull          = 0x05 // 1byte类型，没有数据
	Amf0MarkerUndefined     = 0x06
	Amf0MarkerReference     = 0x07
	Amf0MarkerEcmaArray     = 0x08 // MixedArray, 1byte类型后是4byte的kv个数, 其他和Object差不多
	Amf0MarkerObjectEnd     = 0x09
	Amf0MarkerArray         = 0x0a // StrictArray
	Amf0MarkerDate          = 0x0b
	Amf0MarkerLongString    = 0x0c
	Amf0MarkerUnSupported   = 0x0d
	Amf0MarkerRecordSet     = 0x0e
	Amf0MarkerXmlDocument   = 0x0f
	Amf0MarkerTypedObject   = 0x10
	Amf0MarkerAcmPlusObject = 0x11 // AMF3 data, Sent by Flash player 9+
)

type AmfInfo struct {
	CmdName        string
	TransactionID  float64
	App            string `amf:"app" json:"app"`
	FlashVer       string `amf:"flashVer" json:"flashVer"`
	SwfUrl         string `amf:"swfUrl" json:"swfUrl"`
	TcUrl          string `amf:"tcUrl" json:"tcUrl"`
	Fpad           bool   `amf:"fpad" json:"fpad"`
	AudioCodecs    int    `amf:"audioCodecs" json:"audioCodecs"`
	VideoCodecs    int    `amf:"videoCodecs" json:"videoCodecs"`
	VideoFunction  int    `amf:"videoFunction" json:"videoFunction"`
	PageUrl        string `amf:"pageUrl" json:"pageUrl"`
	ObjectEncoding int    `amf:"objectEncoding" json:"objectEncoding"`
	Type           string
}

type Object map[string]interface{}

// AMF是Adobe开发的二进制通信协议, 有两种版本 AMF0 和 AMF3
// 序列化转结构化 AmfUnmarshal();  结构化转序列化 AmfMarshal();
func AmfHandle(s *Stream, c *Chunk) error {
	r := bytes.NewReader(c.MsgData)
	vs, err := AmfUnmarshal(r) // 序列化转结构化
	if err != nil && err != io.EOF {
		log.Println(err)
		return err
	}
	log.Printf("Amf Marshal %#v", vs)

	switch vs[0].(string) {
	case "connect":
		if err = AmfConnectHandle(s, vs); err != nil {
			return err
		}
		if err = AmfConnectResponse(s, c); err != nil {
			return err
		}
	case "releaseStream":
		return nil
	case "FCPublish":
		return nil
	case "createStream":
		/*
			if err = s.AmfCreateStream(vs[1:]); err != nil {
				return err
			}
			if err = s.AmfCreateStreamResp(c); err != nil {
				return err
			}
		*/
	case "publish":
		/*
			if err = s.AmfPublishOrPlay(vs[1:]); err != nil {
				return err
			}
			if err = s.AmfPublishResp(c); err != nil {
				return err
			}
			s.HandleMessageDone = true
			s.isPublisher = true
		*/
	case "play":
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
	case "FCUnpublish":
	case "deleteStream":
	default:
		err = fmt.Errorf("Untreated AmfCmd", vs[0].(string))
		return err
	}
	return nil
}

/////////////////////////////////////////////////////////////////
// amf decode
/////////////////////////////////////////////////////////////////
func AmfUnmarshal(r io.Reader) (vs []interface{}, err error) {
	var v interface{}
	for {
		log.Println("------")
		v, err = AmfDecode(r)
		if err != nil {
			log.Println(err)
			break
		}
		vs = append(vs, v)
	}
	return vs, err
}

func AmfDecode(r io.Reader) (interface{}, error) {
	t, err := ReadUint8(r)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println("AmfType", t)

	switch t {
	case Amf0MarkerNumber:
		return Amf0DecodeNumber(r)
	case Amf0MarkerBoolen:
		return Amf0DecodeBoolean(r)
	case Amf0MarkerString:
		return Amf0DecodeString(r)
	case Amf0MarkerObject:
		return Amf0DecodeObject(r)
	case Amf0MarkerNull:
		return Amf0DecodeNull(r)
	case Amf0MarkerEcmaArray:
		return Amf0DecodeEcmaArray(r)
	}
	err = fmt.Errorf("Untreated AmfType %d", t)
	log.Println(err)
	return nil, err
}

func Amf0DecodeNumber(r io.Reader) (float64, error) {
	var ret float64
	err := binary.Read(r, binary.BigEndian, &ret)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return 0, err
	}
	log.Println(ret)
	return ret, nil
}

func Amf0DecodeBoolean(r io.Reader) (bool, error) {
	var ret bool
	err := binary.Read(r, binary.BigEndian, &ret)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return false, err
	}
	log.Println(ret)
	return ret, nil
}

func Amf0DecodeString(r io.Reader) (string, error) {
	len, err := ReadUint32(r, 2, BE)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return "", err
	}

	ret, _ := ReadString(r, len)
	log.Println(len, ret)
	return ret, nil
}

func Amf0DecodeObject(r io.Reader) (Object, error) {
	ret := make(Object)
	for {
		// 00 00 09
		len, _ := ReadUint32(r, 2, BE)
		if len == 0 {
			ReadUint8(r)
			break
		}

		key, _ := ReadString(r, len)
		log.Println(key)

		value, err := AmfDecode(r)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		ret[key] = value
	}
	log.Printf("%#v", ret)
	return ret, nil
}

func Amf0DecodeNull(r io.Reader) (interface{}, error) {
	return nil, nil
}

func Amf0DecodeEcmaArray(r io.Reader) (Object, error) {
	len, err := ReadUint32(r, 4, BE)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return nil, err
	}
	log.Println("Amf EcmaArray len", len)

	ret, err := Amf0DecodeObject(r)
	if err != nil {
		log.Println(err)
		if err != io.EOF {
			log.Println(err)
		}
		return nil, err
	}
	log.Printf("%#v", ret)
	return ret, nil
}

/////////////////////////////////////////////////////////////////
// amf encode
/////////////////////////////////////////////////////////////////
func AmfMarshal(args ...interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	for _, v := range args {
		if _, err := AmfEncode(buf, v); err != nil {
			log.Println(err)
			return nil, err
		}
	}
	return buf.Bytes(), nil
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
	err := fmt.Errorf("Untreated Amf0Marker %s", val.Kind())
	log.Println(err)
	return 0, err
}

func Amf0EncodeNull(buf io.Writer) (int, error) {
	b := []byte{Amf0MarkerNull}
	n, err := buf.Write(b)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return n, nil
}

func Amf0EncodeString(buf io.Writer, v string, wType bool) (int, error) {
	var n int
	if wType {
		b := []byte{Amf0MarkerString}
		buf.Write(b)
		n += 1
	}

	l := uint32(len(v))
	WriteUint32(buf, BE, l, 2)
	n += 2

	m, err := buf.Write([]byte(v))
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return n + m, nil
}

func Amf0EncodeBool(buf io.Writer, v bool) (int, error) {
	var n int
	b := []byte{Amf0MarkerBoolen}
	buf.Write(b)
	n += 1

	b[0] = 0x00
	if v {
		b[0] = 0x01
	}

	m, err := buf.Write(b)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return n + m, nil
}

func Amf0EncodeNumber(buf io.Writer, v float64) (int, error) {
	var n int
	b := []byte{Amf0MarkerNumber}
	buf.Write(b)
	n += 1

	err := binary.Write(buf, binary.BigEndian, &v)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return n + 8, nil
}

func Amf0EncodeObject(buf io.Writer, o Object) (int, error) {
	var n, m int
	var err error
	b := []byte{Amf0MarkerObject}
	buf.Write(b)
	n += 1

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

	b[0] = Amf0MarkerObjectEnd
	buf.Write(b)
	return n + 1, nil
}

/////////////////////////////////////////////////////////////////
// amf command handle
/////////////////////////////////////////////////////////////////
func CreateMessage(TypeId, Len uint32, Data []byte) Chunk {
	// fmt: 控制Message Header的类型, 0表示11字节, 1表示7字节, 2表示3字节, 3表示0字节
	// csid: 0表示2字节形式, 1表示3字节形式, 2用于协议控制消息和命令消息, 3-65599表示块流id
	return Chunk{
		Fmt:         0,
		Csid:        2,
		Timestamp:   0,
		MsgLength:   Len,
		MsgTypeId:   TypeId,
		MsgStreamId: 0,
		MsgData:     Data,
	}
}

func AmfConnectHandle(s *Stream, vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case string:
			s.AmfInfo.CmdName = v.(string)
		case float64:
			s.AmfInfo.TransactionID = v.(float64)
		case Object:
			o := v.(Object)
			if i, ok := o["app"]; ok {
				s.AmfInfo.App = i.(string)
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
			if i, ok := o["type"]; ok {
				s.AmfInfo.Type = i.(string)
			}
		}
	}
	log.Printf("%#v", s.AmfInfo)
	return nil
}

func AmfConnectResponse(s *Stream, c *Chunk) error {
	// 1 Window Acknowledge Size
	// 2 Set Peer BandWidth
	// 3 Set ChunkSize
	// 4 User Control(StreamBegin)
	// 5 Command Message (_result- connect response)
	log.Println("Send Window Acknowledge Size")
	d := Uint32ToByte(2500000, nil, BE) // 33554432
	rc := CreateMessage(MsgTypeIdWindowAckSize, 4, d)
	MessageSplit(s, &rc)

	log.Println("Set Peer BandWidth")
	d = make([]byte, 5)
	Uint32ToByte(2500000, d[:4], BE)
	d[4] = 2 // Limit Type: 0 is Hard, 1 is Soft, 2 is Dynamic
	rc = CreateMessage(MsgTypeIdSetPeerBandwidth, 5, d)
	MessageSplit(s, &rc)

	log.Println("Set ChunkSize")
	d = Uint32ToByte(1024, nil, BE)
	rc = CreateMessage(MsgTypeIdSetChunkSize, 4, d)
	MessageSplit(s, &rc)
	// 这里不能设置为1024, 因为下面发送要用到这个值，对方还是128
	//s.ChunkSize = 1024

	rsps := make(Object)
	rsps["fmsVer"] = "FMS/3,0,1,123"
	rsps["capabilities"] = 31
	info := make(Object)
	info["level"] = "status"
	info["code"] = "NetConnection.Connect.Success"
	info["description"] = "Connection succeeded."
	info["objectEncoding"] = s.AmfInfo.ObjectEncoding
	log.Println(rsps, info)

	d, _ = AmfMarshal("_result", 1, rsps, info) // 结构化转序列化
	log.Println(d)

	rc = CreateMessage(MsgTypeIdCmdAmf0, uint32(len(d)), d)
	rc.Csid = c.Csid
	rc.MsgStreamId = c.MsgStreamId
	MessageSplit(s, &rc)
	return nil
}

///////////////////////////////////////////////////////////

/*
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
		vs, err := AmfMarshal(bytes.NewReader(msg.Data))
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

func (s *Stream) HandleCommandMessage(msg *Chunk) error {
	r := bytes.NewReader(msg.Data)
	vs, err := AmfMarshal(r)
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
			if err = s.AmfPublishOrPlay(vs[1:]); err != nil {
				return err
			}
			if err = s.AmfPlayResp(msg); err != nil {
				return err
			}
			s.HandleMessageDone = true
			s.isPublisher = false
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

func main() {
	//TestSize()
	//TestMStreams()
	go RtmpServer(fmt.Sprintf("%s:%s", IfaceIP("eth1"), "1935"))
	http.HandleFunc("/", HttpServer)
	http.ListenAndServe(":8080", nil)
}

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
*/
