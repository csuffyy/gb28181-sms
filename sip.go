package main

import (
	"log"
	"net"
)

func SipHandler(c net.Conn) {
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("len: %d, data: %s", n, string(buf))
}

func SipServer() {
	log.Println("start rtp listen on", conf.Gb28181.SipListen)
	l, err := net.Listen("tcp", conf.Gb28181.SipListen)
	if err != nil {
		log.Fatalln(err)
	}

	for {
		c, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println("---------->> new tcp(sip) connect")
		log.Println("RemoteAddr:", c.RemoteAddr().String())

		go SipHandler(c)
	}
}