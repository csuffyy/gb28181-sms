package main

import "log"

func main() {
	var b []byte
	b = make([]byte, 1)
	b[0] = 0x0
	//b[1] = 0x1
	log.Printf("%x", b)
}
