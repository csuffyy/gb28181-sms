package main

import "log"

func changeByte(b []byte) {
	b[0] = 0x1
}

func main() {
	var b []byte
	b = make([]byte, 1)
	b[0] = 0x3
	log.Printf("%x", b)
	//changeByte(b)
	//log.Printf("%x", b)
	x := b[0] & 0x1
	y := b[0] | 0x1
	log.Printf("%x, %x, %x", b, x, y)
}
