package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"livego/protocol/amf"
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
	TransactionId  float64
	App            string `amf:"app" json:"app"`
	FlashVer       string `amf:"flashVer" json:"flashVer"`
	SwfUrl         string `amf:"swfUrl" json:"swfUrl"`
	TcUrl          string `amf:"tcUrl" json:"tcUrl"`
	Fpad           bool   `amf:"fpad" json:"fpad"`
	AudioCodecs    int    `amf:"audioCodecs" json:"audioCodecs"`
	VideoCodecs    int    `amf:"videoCodecs" json:"videoCodecs"`
	VideoFunction  int    `amf:"videoFunction" json:"videoFunction"`
	PageUrl        string `amf:"pageUrl" json:"pageUrl"`
	ObjectEncoding int    // 0 is AMF0, 3 is AMF3
	Type           string
	PublishName    string
	PublishType    string  // live/ record/ append
	StreamName     string  // play cmd use
	Start          float64 // play cmd use
	Duration       float64 // play cmd use, live is -1
	Reset          bool    // play cmd use
}

type Object map[string]interface{}

// AMF是Adobe开发的二进制通信协议, 有两种版本 AMF0 和 AMF3
// 序列化转结构化 AmfUnmarshal();  结构化转序列化 AmfMarshal();
func AmfHandle(s *Stream, c *Chunk) error {
	r := bytes.NewReader(c.MsgData)
	vs, err := AmfUnmarshal(s, r) // 序列化转结构化
	if err != nil && err != io.EOF {
		s.log.Println(err)
		return err
	}
	s.log.Printf("Amf Unmarshal %#v", vs)

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
		if err = AmfCreateStreamHandle(s, vs); err != nil {
			return err
		}
		if err = AmfCreateStreamResponse(s, c); err != nil {
			return err
		}
	case "publish":
		if err = AmfPublishHandle(s, vs); err != nil {
			return err
		}
		if err = AmfPublishResponse(s, c); err != nil {
			return err
		}
		s.IsPublisher = true
		s.MessageHandleDone = true
	case "play":
		if err = AmfPlayHandle(s, vs); err != nil {
			return err
		}
		if err = AmfPlayResponse(s, c); err != nil {
			return err
		}
		s.IsPublisher = false
		s.MessageHandleDone = true
	//case "FCUnpublish":
	//case "deleteStream":
	case "getStreamLength": // play 交互出现
		return nil
	default:
		err = fmt.Errorf("Untreated AmfCmd %s", vs[0].(string))
		s.log.Println(err)
		return err
	}
	return nil
}

/////////////////////////////////////////////////////////////////
// amf decode
/////////////////////////////////////////////////////////////////
func AmfUnmarshal(s *Stream, r io.Reader) (vs []interface{}, err error) {
	var v interface{}
	for {
		s.log.Println("------")
		v, err = AmfDecode(s, r)
		if err != nil {
			s.log.Println(err)
			break
		}
		vs = append(vs, v)
	}
	return vs, err
}

func AmfDecode(s *Stream, r io.Reader) (interface{}, error) {
	t, err := ReadUint8(r)
	if err != nil {
		s.log.Println(err)
		return nil, err
	}
	s.log.Println("AmfType", t)

	switch t {
	case Amf0MarkerNumber:
		return Amf0DecodeNumber(s, r)
	case Amf0MarkerBoolen:
		return Amf0DecodeBoolean(s, r)
	case Amf0MarkerString:
		return Amf0DecodeString(s, r)
	case Amf0MarkerObject:
		return Amf0DecodeObject(s, r)
	case Amf0MarkerNull:
		return Amf0DecodeNull(s, r)
	case Amf0MarkerEcmaArray:
		return Amf0DecodeEcmaArray(s, r)
	}
	err = fmt.Errorf("Untreated AmfType %d", t)
	s.log.Println(err)
	return nil, err
}

func Amf0DecodeNumber(s *Stream, r io.Reader) (float64, error) {
	var ret float64
	err := binary.Read(r, binary.BigEndian, &ret)
	if err != nil {
		if err != io.EOF {
			s.log.Println(err)
		}
		return 0, err
	}
	s.log.Println(ret)
	return ret, nil
}

func Amf0DecodeBoolean(s *Stream, r io.Reader) (bool, error) {
	var ret bool
	err := binary.Read(r, binary.BigEndian, &ret)
	if err != nil {
		if err != io.EOF {
			s.log.Println(err)
		}
		return false, err
	}
	s.log.Println(ret)
	return ret, nil
}

func Amf0DecodeString(s *Stream, r io.Reader) (string, error) {
	len, err := ReadUint32(r, 2, BE)
	if err != nil {
		if err != io.EOF {
			s.log.Println(err)
		}
		return "", err
	}

	ret, _ := ReadString(r, len)
	s.log.Println(len, ret)
	return ret, nil
}

func Amf0DecodeObject(s *Stream, r io.Reader) (Object, error) {
	ret := make(Object)
	for {
		// 00 00 09
		len, _ := ReadUint32(r, 2, BE)
		if len == 0 {
			ReadUint8(r)
			break
		}

		key, _ := ReadString(r, len)
		s.log.Println(key)

		value, err := AmfDecode(s, r)
		if err != nil {
			s.log.Println(err)
			return nil, err
		}
		ret[key] = value
	}
	//s.log.Printf("%#v", ret)
	return ret, nil
}

func Amf0DecodeNull(s *Stream, r io.Reader) (interface{}, error) {
	return nil, nil
}

func Amf0DecodeEcmaArray(s *Stream, r io.Reader) (Object, error) {
	len, err := ReadUint32(r, 4, BE)
	if err != nil {
		if err != io.EOF {
			s.log.Println(err)
		}
		return nil, err
	}
	s.log.Println("Amf EcmaArray len", len)

	ret, err := Amf0DecodeObject(s, r)
	if err != nil {
		s.log.Println(err)
		if err != io.EOF {
			s.log.Println(err)
		}
		return nil, err
	}
	//s.log.Printf("%#v", ret)
	return ret, nil
}

/////////////////////////////////////////////////////////////////
// amf encode
/////////////////////////////////////////////////////////////////
func AmfMarshal(s *Stream, args ...interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	for _, v := range args {
		if _, err := AmfEncode(s, buf, v); err != nil {
			s.log.Println(err)
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func AmfEncode(s *Stream, buf io.Writer, v interface{}) (int, error) {
	if v == nil {
		return Amf0EncodeNull(s, buf)
	}

	val := reflect.ValueOf(v)
	s.log.Println(v, val.Kind())
	switch val.Kind() {
	case reflect.String:
		return Amf0EncodeString(s, buf, val.String(), true)
	case reflect.Bool:
		return Amf0EncodeBool(s, buf, val.Bool())
	case reflect.Int:
		return Amf0EncodeNumber(s, buf, float64(val.Int()))
	case reflect.Uint32:
		return Amf0EncodeNumber(s, buf, float64(val.Uint()))
	case reflect.Float32, reflect.Float64:
		return Amf0EncodeNumber(s, buf, float64(val.Float()))
	case reflect.Map:
		return Amf0EncodeObject(s, buf, v.(Object))
	}
	err := fmt.Errorf("Untreated Amf0Marker %s", val.Kind())
	s.log.Println(err)
	return 0, err
}

func Amf0EncodeNull(s *Stream, buf io.Writer) (int, error) {
	b := []byte{Amf0MarkerNull}
	n, err := buf.Write(b)
	if err != nil {
		s.log.Println(err)
		return 0, err
	}
	return n, nil
}

func Amf0EncodeString(s *Stream, buf io.Writer, v string, wType bool) (int, error) {
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
		s.log.Println(err)
		return 0, err
	}
	return n + m, nil
}

func Amf0EncodeBool(s *Stream, buf io.Writer, v bool) (int, error) {
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
		s.log.Println(err)
		return 0, err
	}
	return n + m, nil
}

func Amf0EncodeNumber(s *Stream, buf io.Writer, v float64) (int, error) {
	var n int
	b := []byte{Amf0MarkerNumber}
	buf.Write(b)
	n += 1

	err := binary.Write(buf, binary.BigEndian, &v)
	if err != nil {
		s.log.Println(err)
		return 0, err
	}
	return n + 8, nil
}

func Amf0EncodeObject(s *Stream, buf io.Writer, o Object) (int, error) {
	var n, m int
	var err error
	b := []byte{Amf0MarkerObject}
	buf.Write(b)
	n += 1

	for k, v := range o {
		m, err = Amf0EncodeString(s, buf, k, false)
		if err != nil {
			s.log.Println(err)
			return 0, err
		}
		n += m

		m, err = AmfEncode(s, buf, v)
		if err != nil {
			s.log.Println(err)
			return 0, err
		}
		n += m
	}

	m, err = Amf0EncodeString(s, buf, "", false)
	if err != nil {
		s.log.Println(err)
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
			s.AmfInfo.TransactionId = v.(float64)
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
	s.log.Printf("%#v", s.AmfInfo)
	return nil
}

func AmfConnectResponse(s *Stream, c *Chunk) error {
	// 1 Window Acknowledge Size
	// 2 Set Peer BandWidth
	// 3 Set ChunkSize
	// 4 User Control(StreamBegin)
	// 5 Command Message (_result- connect response)
	s.log.Println("Send Window Acknowledge Size")
	d := Uint32ToByte(2500000, nil, BE) // 33554432
	rc := CreateMessage(MsgTypeIdWindowAckSize, 4, d)
	MessageSplit(s, &rc)

	s.log.Println("Set Peer BandWidth")
	d = make([]byte, 5)
	Uint32ToByte(2500000, d[:4], BE)
	d[4] = 2 // Limit Type: 0 is Hard, 1 is Soft, 2 is Dynamic
	rc = CreateMessage(MsgTypeIdSetPeerBandwidth, 5, d)
	MessageSplit(s, &rc)

	s.log.Println("Set ChunkSize")
	d = Uint32ToByte(1024, nil, BE)
	rc = CreateMessage(MsgTypeIdSetChunkSize, 4, d)
	MessageSplit(s, &rc)
	// 这里必须设置为1024, 因为上面已通知对方ChunkSize=1024
	s.ChunkSize = 1024

	s.log.Println("Send Command Message")
	rsps := make(Object)
	rsps["fmsVer"] = "FMS/3,0,1,123"
	rsps["capabilities"] = 31
	info := make(Object)
	info["level"] = "status"
	info["code"] = "NetConnection.Connect.Success"
	info["description"] = "Connection succeeded."
	info["objectEncoding"] = s.AmfInfo.ObjectEncoding
	s.log.Println(rsps, info)

	d, _ = AmfMarshal(s, "_result", 1, rsps, info) // 结构化转序列化
	s.log.Println(d)

	rc = CreateMessage(MsgTypeIdCmdAmf0, uint32(len(d)), d)
	rc.Csid = c.Csid
	rc.MsgStreamId = c.MsgStreamId
	MessageSplit(s, &rc)
	return nil
}

func AmfCreateStreamHandle(s *Stream, vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case string:
			s.AmfInfo.CmdName = v.(string)
		case float64:
			s.AmfInfo.TransactionId = v.(float64)
		}
	}
	s.log.Printf("%#v", s.AmfInfo)
	return nil
}

func AmfCreateStreamResponse(s *Stream, c *Chunk) error {
	// 1 Command Message (_result- createStream response)
	s.log.Println("Send Command Message")
	d, _ := AmfMarshal(s, "_result", s.AmfInfo.TransactionId, nil, c.MsgStreamId)
	s.log.Println(d)

	rc := CreateMessage(MsgTypeIdCmdAmf0, uint32(len(d)), d)
	rc.Csid = c.Csid
	rc.MsgStreamId = c.MsgStreamId
	MessageSplit(s, &rc)
	return nil
}

func AmfPublishHandle(s *Stream, vs []interface{}) error {
	for k, v := range vs {
		switch v.(type) {
		case string:
			if k == 0 {
				s.AmfInfo.CmdName = v.(string)
			} else if k == 3 {
				s.AmfInfo.PublishName = v.(string)
			} else if k == 4 {
				s.AmfInfo.PublishType = v.(string)
			}
		case float64:
			s.AmfInfo.TransactionId = v.(float64)
		case amf.Object:
			s.log.Println("Untreated AmfType")
		}
	}
	s.log.Printf("%#v", s.AmfInfo)
	return nil
}

func AmfPublishResponse(s *Stream, c *Chunk) error {
	info := make(Object)
	info["level"] = "status"
	info["code"] = "NetStream.Publish.Start"
	info["description"] = "Start publising."

	d, _ := AmfMarshal(s, "onStatus", 0, nil, info) // 结构化转序列化
	s.log.Println(d)

	rc := CreateMessage(MsgTypeIdCmdAmf0, uint32(len(d)), d)
	rc.Csid = c.Csid
	rc.MsgStreamId = c.MsgStreamId
	MessageSplit(s, &rc)
	return nil
}

func AmfPlayHandle(s *Stream, vs []interface{}) error {
	for k, v := range vs {
		switch v.(type) {
		case string:
			if k == 0 {
				s.AmfInfo.CmdName = v.(string)
			} else if k == 3 {
				s.AmfInfo.StreamName = v.(string)
			}
		case float64:
			if k == 1 {
				s.AmfInfo.TransactionId = v.(float64)
			} else if k == 4 {
				s.AmfInfo.Start = v.(float64)
			} else if k == 5 {
				s.AmfInfo.Duration = v.(float64)
			}
		case amf.Object:
			s.log.Println("Untreated AmfType")
		case bool:
			s.AmfInfo.Reset = v.(bool)
		}
	}
	s.log.Printf("%#v", s.AmfInfo)
	return nil
}

func AmfPlayResponse(s *Stream, c *Chunk) error {
	return nil
}
