package main

import (
	"io"
	"log"
	"unsafe"
)

// 以小端方式写，以小端方式读
// 以大端方式写，以大端方式读
// 0x11223344	0x11 为高位，0x44为低位
// b[4]			b[0] 为低地址, b[3] 为高地址
// uint32		b[0] b[1] b[2] b[3]
// 0x11223344	0x44 0x33 0x22 0x11 小端 低位在低地址,低地址放低位
// 0x11223344	0x11 0x22 0x33 0x44 大端 低位在高地址,低地址放高位
// /usr/local/go/src/encoding/binary/binary.go

type ByteOrder string

const (
	BE ByteOrder = "BigEndian"
	LE ByteOrder = "LittleEndian"
)

func TestSize() {
	var f32 float32
	var f64 float64
	var b bool
	log.Println("float32 size:", unsafe.Sizeof(f32))
	log.Println("float64 size:", unsafe.Sizeof(f64))
	log.Println("bool size:", unsafe.Sizeof(b))
}

func GetByteOrder() string {
	u16 := uint16(0x1122) // 0x11为高位, 0x22为低位
	u8 := uint8(u16)      // u8[0]为低地址, u8[1]为高地址
	//log.Println(unsafe.Sizeof(u16), unsafe.Sizeof(u8))
	if u8 == 0x22 { // 低位在低地址, 为小端
		return "LittleEndian"
	}
	return "BigEndian"
}

// []byte 以大/小端字节序 转 uintX
func ByteToUint16(b []byte, bo ByteOrder) uint16 {
	n := len(b)
	if n > 2 {
		n = 2
	}
	var u uint16
	for i := 0; i < n; i++ {
		if bo == BE {
			u = u<<8 + uint16(b[i])
		} else {
			u += uint16(b[i]) << uint16(i*8)
		}
	}
	return u
}

func ByteToUint32(b []byte, bo ByteOrder) uint32 {
	n := len(b)
	if n > 4 {
		n = 4
	}
	var u uint32
	for i := 0; i < n; i++ {
		if bo == BE {
			u = u<<8 + uint32(b[i])
		} else {
			u += uint32(b[i]) << uint32(i*8)
		}
	}
	return u
}

func ByteToUint64(b []byte, bo ByteOrder) uint64 {
	n := len(b)
	if n > 8 {
		n = 8
	}
	var u uint64
	for i := 0; i < n; i++ {
		if bo == BE {
			u = u<<8 + uint64(b[i])
		} else {
			u += uint64(b[i]) << uint64(i*8)
		}
	}
	return u
}

// uintX 以大/小端字节序 转 []byte
func Uint16ToByte(u uint16, b []byte, bo ByteOrder) []byte {
	bb := make([]byte, 2)
	for i := 0; i < 2; i++ {
		if bo == BE {
			bb[i] = byte(u >> uint16((4-i-1)*8))
		} else {
			bb[i] = byte(u >> uint16(i*8))
		}
	}
	if b != nil {
		copy(b, bb)
	}
	return bb
}

func Uint32ToByte(u uint32, b []byte, bo ByteOrder) []byte {
	bb := make([]byte, 4)
	for i := 0; i < 4; i++ {
		if bo == BE {
			bb[i] = byte(u >> uint32((4-i-1)*8))
		} else {
			bb[i] = byte(u >> uint32(i*8))
		}
	}
	if b != nil {
		copy(b, bb)
	}
	return bb
}

func Uint64ToByte(u uint64, b []byte, bo ByteOrder) []byte {
	bb := make([]byte, 8)
	for i := 0; i < 8; i++ {
		if bo == BE {
			bb[i] = byte(u >> uint64((4-i-1)*8))
		} else {
			bb[i] = byte(u >> uint64(i*8))
		}
	}
	if b != nil {
		copy(b, bb)
	}
	return bb
}

// 从 socket中 读取 n个字节, 以大/小端字节序 转 uintX
func ReadByte(r io.Reader, n uint32) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return b, err
	}
	return b, nil
}

func ReadString(r io.Reader, n uint32) (string, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return "", err
	}
	return string(b), nil
}

func ReadUint8(r io.Reader) (uint8, error) {
	b, err := ReadByte(r, 1)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return 0, err
	}
	return uint8(b[0]), nil
}

func ReadUint16(r io.Reader, n uint32, bo ByteOrder) (uint16, error) {
	b, err := ReadByte(r, n)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return 0, err
	}
	return ByteToUint16(b, bo), nil
}

func ReadUint32(r io.Reader, n uint32, bo ByteOrder) (uint32, error) {
	b, err := ReadByte(r, n)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return 0, err
	}
	return ByteToUint32(b, bo), nil
}

func ReadUint64(r io.Reader, n uint32, bo ByteOrder) (uint64, error) {
	b, err := ReadByte(r, n)
	if err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return 0, err
	}
	return ByteToUint64(b, bo), nil
}

// uintX 以大/小端字节序 写入到 socket中
func WriteByte(w io.Writer, b []byte) (int, error) {
	n, err := w.Write(b)
	if err != nil {
		log.Println(err)
		return n, err
	}
	return n, nil
}

func WriteString(w io.Writer, s string) (int, error) {
	n, err := w.Write([]byte(s))
	if err != nil {
		log.Println(err)
		return n, err
	}
	return n, nil
}

func WriteUint8(w io.Writer, u uint8) error {
	_, err := WriteByte(w, []byte{u})
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func WriteUint16(w io.Writer, bo ByteOrder, u, n uint16) error {
	var b, bb []byte
	b = Uint16ToByte(u, nil, bo)
	if bo == BE {
		bb = b[2-n:]
	} else {
		bb = b[:n]
	}
	_, err := WriteByte(w, bb)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func WriteUint32(w io.Writer, bo ByteOrder, u, n uint32) error {
	var b, bb []byte
	b = Uint32ToByte(u, nil, bo)
	if bo == BE {
		bb = b[4-n:]
	} else {
		bb = b[:n]
	}
	//log.Println(len(bb), bb)
	_, err := WriteByte(w, bb)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func WriteUint64(w io.Writer, bo ByteOrder, u, n uint64) error {
	var b, bb []byte
	b = Uint64ToByte(u, nil, bo)
	if bo == BE {
		bb = b[8-n:]
	} else {
		bb = b[:n]
	}
	_, err := WriteByte(w, bb)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}
