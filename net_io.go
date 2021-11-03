package main

import (
	"io"
	"log"
)

// /usr/local/go/src/encoding/binary/binary.go
func ByteToUint32BE(b []byte) uint32 {
	ret := uint32(0)
	for i := 0; i < len(b); i++ {
		ret = ret<<8 + uint32(b[i])
	}
	return ret
}

func Uint32ToByteBE(b []byte, v uint32, len int) {
	for i := 0; i < len; i++ {
		b[i] = byte(v >> uint32((len-i-1)*8))
	}
}

func Uint32ToByteLE(b []byte, v uint32, len int) {
	for i := 0; i < len; i++ {
		b[i] = byte(v >> uint32(i*8))
	}
}

func ReadByteToUint32BE(r io.Reader, n uint32) (uint32, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return 0, err
	}
	return ByteToUint32BE(b), nil
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
