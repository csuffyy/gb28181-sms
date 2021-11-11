package main

import (
	"io"
	"log"
)

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
