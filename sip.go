package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"utils"
)

/*
协议版本			GB/T28181-2016
SIP服务器ID			11000000122000000034
SIP服务器域			1100000012
SIP服务器地址		10.3.220.68
SIP服务器端口		62097
SIP用户名			11010000121310000034
SIP用户认证ID		11010000121310000034
密码				123456
注册有效期			3600 秒
心跳周期			60 秒
本地SIP端口			5060
注册间隔			60 秒
最大心跳超时次数	3
*/
// 摄像头	10.3.220.151	5060
// DCN		10.3.220.68		62097 50280

// 流媒体	172.20.25.20	62097
// tcpdump -nnvvv -i eth0 port 62097 -w sipSms.pcap
// scp root@172.20.25.20:/root/sipSms.pcap .

// sip		172.20.25.28	50280
// ssh root@172.20.25.28	Lty20@Ltsk21
// tcpdump -nnvvv -i eth0 host 10.3.220.151 -w sipSvr.pcap
// scp root@172.20.25.28:/root/sipSvr.pcap .

var Sip1Rsps = "SIP/2.0 401 Unauthorized\r\n" +
	"Via: SIP/2.0/TCP %s:%s;rport=%s;received=%s;branch=%s\r\n" +
	"From: <sip:11010000121310000034@1100000012>;tag=%s\r\n" +
	"To: <sip:11010000121310000034@1100000012>;tag=%s\r\n" +
	"Call-ID: %s\r\n" +
	"CSeq: 1 REGISTER\r\n" +
	"WWW-Authenticate: Digest realm=\"1100000012\",nonce=\"%s\",opaque=\"%s\",algorithm=md5\r\n" +
	"Content-Length:  0\r\n\r\n"

var Sip2Rsps = "SIP/2.0 200 OK\r\n" +
	"Via: SIP/2.0/TCP %s:%s;rport=%s;received=%s;branch=%s\r\n" +
	"From: <sip:11010000121310000034@1100000012>;tag=%s\r\n" +
	"To: <sip:11010000121310000034@1100000012>;tag=%s\r\n" +
	"Call-ID: %s\r\n" +
	"CSeq: %d REGISTER\r\n" +
	"Data: %s\r\n" +
	"Expires: 3600\r\n" +
	"Content-Length:  0\r\n\r\n"

var Sip4Rsps = "SIP/2.0 200 OK\r\n" +
	"Via: SIP/2.0/TCP %s:%s;rport=%s;received=%s;branch=%s\r\n" +
	"From: <sip:11010000121310000034@1100000012>;tag=%s\r\n" +
	"To: <sip:11010000121310000034@1100000012>;tag=%s\r\n" +
	"Call-ID: %s\r\n" +
	"CSeq: %s MESSAGE\r\n" +
	"Content-Length:  0\r\n\r\n"

var Sip1Rqst = "MESSAGE sip:11010000121310000034@10.3.220.151:5060 SIP/2.0\r\n" +
	"Via: SIP/2.0/TCP 10.3.220.68:62097;rport;branch=z9hG4bKPjKGSI94O1i7pQoYqcuJnNBdIDWGI3TyR.\r\n" +
	"From: <sip:11000000122000000034@1100000012>;tag=u1TsYkW1MPmX6L5Vz0KD2mdIf8lexOTB\r\n" +
	"To: <sip:11010000121310000034@1100000012>\r\n" +
	"Call-ID: IMg2pPgKOBdDB3SowCcfiNeUsXjdMuRB\r\n" +
	"CSeq: 49715 MESSAGE\r\n" +
	"Max-Forwards: 70\r\n" +
	"Content-Type: Application/MANSCDP+xml\r\n" +
	"Content-Length:   141\r\n\r\n" +
	"<?xml version=\"1.0\" ?>\n" +
	"<Query>\n" +
	"    <CmdType>DeviceInfo</CmdType>\n" +
	"    <SN>1</SN>\n" +
	"    <DeviceID>42010000121310000000</DeviceID>\n" +
	"</Query>\n"

var Sip2Rqst = "MESSAGE sip:11010000121310000034@10.3.220.151:5060 SIP/2.0\r\n" +
	"Via: SIP/2.0/TCP 10.3.220.68:62097;rport;branch=z9hG4bKPjfnUTD6prc4w82Jd2vxFy16E8LV.kZZZz\r\n" +
	"From: <sip:11000000122000000034@1100000012>;tag=hn3zMtTNiHYmNLr3J9bm5TyUGMJuhp3n\r\n" +
	"To: <sip:11010000121310000034@1100000012>\r\n" +
	"Call-ID: PiJRk4MOfpmVQZR8ub3XBdE36l3AIwih\r\n" +
	"CSeq: 40628 MESSAGE\r\n" +
	"Max-Forwards: 70\r\n" +
	"Content-Type: Application/MANSCDP+xml\r\n" +
	"Content-Length:   138\r\n\r\n" +
	"<?xml version=\"1.0\" ?>\n" +
	"<Query>\n" +
	"    <CmdType>Catalog</CmdType>\n" +
	"    <SN>1</SN>\n" +
	"    <DeviceID>42010000121310000000</DeviceID>\n" +
	"</Query>\n"

type Sip1 struct {
	Ip       string //
	Port     string //
	Received string
	Branch   string //
	FromTag  string //
	ToTag    string
	CallId   string //
	CSeq     string //
	Nonce    string
	Opaque   string
}

func GetSip1Info(s string) Sip1 {
	line := strings.Split(s, "\n")
	lineNum := len(line)

	s1 := Sip1{}
	s1.Received = "172.20.25.20"
	s1.ToTag = "z9hG4bK2078339622"
	s1.Nonce = "43b4f4162cfa5a35"
	s1.Opaque = "040feeef38b042e6"

	var aa []string
	for i := 0; i < lineNum; i++ {
		if strings.Contains(string(line[i]), "Via:") {
			aa = strings.Split(line[i], " ")
			aa = strings.Split(aa[2], ";")
			aa = strings.Split(aa[0], ":")
			s1.Ip = aa[0]
			s1.Port = aa[1]

			aa = strings.Split(line[i], "=")
			s1.Branch = strings.Replace(aa[1], "\r", "", -1)
		} else if strings.Contains(string(line[i]), "From:") {
			aa = strings.Split(line[i], "=")
			s1.FromTag = strings.Replace(aa[1], "\r", "", -1)
		} else if strings.Contains(string(line[i]), "Call-ID:") {
			aa = strings.Split(line[i], ":")
			s1.CallId = strings.Replace(aa[1], " ", "", -1)
			s1.CallId = strings.Replace(s1.CallId, "\r", "", -1)
		}
	}
	return s1
}

func SipRegister1(c net.Conn, s string) {
	s1 := GetSip1Info(s)
	//log.Printf("%#v", s1)

	sip1Rsps := fmt.Sprintf(Sip1Rsps, s1.Ip, s1.Port, s1.Port, s1.Received, s1.Branch, s1.FromTag, s1.ToTag, s1.CallId, s1.Nonce, s1.Opaque)

	n, err := c.Write([]byte(sip1Rsps))
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("sendLen: %d, sendData: %s", n, sip1Rsps)
}

func GetSip2Info(s string) Sip1 {
	line := strings.Split(s, "\n")
	lineNum := len(line)

	s1 := Sip1{}
	s1.Received = "172.20.25.20"
	s1.ToTag = "z9hG4bK360295267"

	var aa []string
	for i := 0; i < lineNum; i++ {
		if strings.Contains(string(line[i]), "Via:") {
			aa = strings.Split(line[i], " ")
			aa = strings.Split(aa[2], ";")
			aa = strings.Split(aa[0], ":")
			s1.Ip = aa[0]
			s1.Port = aa[1]

			aa = strings.Split(line[i], "=")
			s1.Branch = strings.Replace(aa[1], "\r", "", -1)
		} else if strings.Contains(string(line[i]), "From:") {
			aa = strings.Split(line[i], "=")
			s1.FromTag = strings.Replace(aa[1], "\r", "", -1)
		} else if strings.Contains(string(line[i]), "Call-ID:") {
			aa = strings.Split(line[i], ":")
			s1.CallId = strings.Replace(aa[1], " ", "", -1)
			s1.CallId = strings.Replace(s1.CallId, "\r", "", -1)
		}
	}
	return s1
}

func SipRegister2(c net.Conn, s string, CSeq int) {
	s1 := GetSip2Info(s)
	if CSeq == 3 {
		s1.ToTag = "z9hG4bK439144480"
	} else if CSeq == 4 {
		s1.ToTag = "z9hG4bK780513006"
	}
	//log.Printf("%#v", s1)

	date := utils.GetYMDHMS1()
	sip2Rsps := fmt.Sprintf(Sip2Rsps, s1.Ip, s1.Port, s1.Port, s1.Received, s1.Branch, s1.FromTag, s1.ToTag, s1.CallId, CSeq, date)

	n, err := c.Write([]byte(sip2Rsps))
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("sendLen: %d, sendData: %s", n, sip2Rsps)
}

func GetSipMsgInfo(s string) Sip1 {
	line := strings.Split(s, "\n")
	lineNum := len(line)

	s1 := Sip1{}
	s1.Received = "172.20.25.20"
	s1.ToTag = "z9hG4bK360295268"

	var aa []string
	for i := 0; i < lineNum; i++ {
		if strings.Contains(string(line[i]), "Via:") {
			aa = strings.Split(line[i], " ")
			aa = strings.Split(aa[2], ";")
			aa = strings.Split(aa[0], ":")
			s1.Ip = aa[0]
			s1.Port = aa[1]

			aa = strings.Split(line[i], "=")
			s1.Branch = strings.Replace(aa[1], "\r", "", -1)
		} else if strings.Contains(string(line[i]), "From:") {
			aa = strings.Split(line[i], "=")
			s1.FromTag = strings.Replace(aa[1], "\r", "", -1)
		} else if strings.Contains(string(line[i]), "Call-ID:") {
			aa = strings.Split(line[i], ":")
			s1.CallId = strings.Replace(aa[1], " ", "", -1)
			s1.CallId = strings.Replace(s1.CallId, "\r", "", -1)
		} else if strings.Contains(string(line[i]), "CSeq:") {
			aa = strings.Split(line[i], " ")
			s1.CSeq = strings.Replace(aa[1], " ", "", -1)
		}
	}
	return s1
}

func SipMessage(c net.Conn, s string) {
	s1 := GetSipMsgInfo(s)
	//log.Printf("%#v", s1)

	//date := utils.GetYMDHMS1()
	sip4Rsps := fmt.Sprintf(Sip4Rsps, s1.Ip, s1.Port, s1.Port, s1.Received, s1.Branch, s1.FromTag, s1.ToTag, s1.CallId, s1.CSeq)

	n, err := c.Write([]byte(sip4Rsps))
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("sendLen: %d, sendData: %s", n, sip4Rsps)
}

func SipRegisterX() {
	log.Println("SipRegisterX()")
}

func SipRqstSend(c net.Conn, rqst string) {
	n, err := c.Write([]byte(rqst))
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("sendLen: %d, sendData: %s", n, rqst)
}

func SipHandler(c net.Conn) {
	i := 0
	for {
		log.Printf("------> sipRecv %d", i)
		i++

		buf := make([]byte, 1024)
		n, err := c.Read(buf)
		if err != nil {
			log.Println(err)
			return
		}
		s := string(buf)
		if n == 4 {
			log.Printf("recvLen: %d, recvData: %x", n, buf[:n])
		} else {
			log.Printf("recvLen: %d, recvData: %s", n, s)

		}

		if strings.Contains(s, "CSeq: 1 REGISTER") {
			SipRegister1(c, s)
		} else if strings.Contains(s, "CSeq: 2 REGISTER") {
			SipRegister2(c, s, 2)
			SipRqstSend(c, Sip1Rqst)
			SipRqstSend(c, Sip2Rqst)
		} else if strings.Contains(s, "CSeq: 3 REGISTER") {
			SipRegister2(c, s, 3)
		} else if strings.Contains(s, "CSeq: 4 REGISTER") {
			SipRegister2(c, s, 4)
		} else if strings.Contains(s, "MESSAGE sip:") {
			SipMessage(c, s)
		} else {
			SipRegisterX()
		}
	}
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
