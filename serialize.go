package main

import (
	"io"
	"log"
)

type ByteOrder string

const (
	BE ByteOrder = "BigEndian"
	LE ByteOrder = "LittleEndian"
)

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

func ByteToUint64BE(b []byte, bo ByteOrder) uint64 {
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
func Uint16ToByte(u uint16, bo ByteOrder) []byte {
	var b []byte
	for i := 0; i < 2; i++ {
		if bo == BE {
			b[i] = byte(u >> uint16((4-i-1)*8))
		} else {
			b[i] = byte(u >> uint16(i*8))
		}
	}
	return b
}

func Uint32ToByteBE(u uint32, bo ByteOrder) []byte {
	var b []byte
	for i := 0; i < 4; i++ {
		if bo == BE {
			b[i] = byte(u >> uint32((4-i-1)*8))
		} else {
			b[i] = byte(u >> uint32(i*8))
		}
	}
	return b
}

func Uint64ToByteBE(u uint64, bo ByteOrder) []byte {
	var b []byte
	for i := 0; i < 8; i++ {
		if bo == BE {
			b[i] = byte(u >> uint64((4-i-1)*8))
		} else {
			b[i] = byte(u >> uint64(i*8))
		}
	}
	return b
}

// 从 socket中 读取 n个字节, 以大/小端字节序 转 uintX
func ReadByte(r io.Reader, n uint8) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	if err != nil {
		log.Println(err)
		return b, err
	}
	return b, nil
}

func ReadByteToUint16(r io.Reader, bo ByteOrder) (uint16, error) {
	b, err := ReadByte(r, 2)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return ByteToUint16(b, bo), nil
}

func ReadByteToUint32(r io.Reader, bo ByteOrder) (uint32, error) {
	b, err := ReadByte(r, 4)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return ByteToUint32(b, bo), nil
}

func ReadByteToUint64(r io.Reader, bo ByteOrder) (uint64, error) {
	b, err := ReadByte(r, 8)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return ByteToUint64(b, bo), nil
}

// uint32 以大/小端字节序 写入到 socket中
