package main

import (
	"fmt"
	"io"
	"log"
)

type ChunkStream struct {
	Format    uint32
	CSID      uint32
	Timestamp uint32
	Length    uint32
	TypeID    uint32
	StreamID  uint32
	timeDelta uint32
	exted     bool
	index     uint32
	remain    uint32
	got       bool
	tmpFromat uint32
	Data      []byte
}

type Chunk struct {
	Fmt       uint32
	Csid      uint32
	Timestamp uint32
	TimeExted bool
	TimeDelta uint32
	Length    uint32
	TypeID    uint32
	StreamID  uint32
	Data      []byte
	Index     uint32
	Remain    uint32
	Full      bool
}

func ChunkMerge(s *Stream, c *Chunk) error {
	var bh uint8 // basic header
	for {
		//binary.Read(s.Conn, binary.BigEndian, &bh)
		u, _ := ReadByteToUint32BE(s.Conn, 1)
		fmt := u >> 6
		csid := u & 0x3f

		// csid: 0 表示 2字节形式, 1 表示 3字节形式, 2 被保留, 3-65599 表示块流id
		switch csid {
		case 0:
			id := ReadByteToUint32LE(s.Conn, 1, 0)
			csid = id + 64
		case 1:
			id := ReadByteToUint32LE(s.Conn, 2, 0)
			csid = id + 64
		}
		log.Println("fmt:", fmt, "csid:", csid) // xxx

		cs, ok := s.chunks[csid]
		if !ok {
			cs = Chunk{}
		}

		cs.Fmt = fmt
		cs.Csid = csid
		if err = cs.ChunkAssmble(s.Conn, s.chunkSize); err != nil {
			log.Println(err)
			return
		}

		s.chunks[csid] = cs
		if cs.Full {
			*c = cs
			log.Println("chunk Full")
			break
		}
	}
	return
}

//////////////////////////////////////////////////////////////////////////

const (
	TAG_AUDIO          = 8
	TAG_VIDEO          = 9
	TAG_SCRIPTDATAAMF0 = 18
	TAG_SCRIPTDATAAMF3 = 0xf
)

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
