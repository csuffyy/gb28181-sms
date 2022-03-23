package main

import (
	"log"
	"os"
)

// go run 会将源码编译到系统TEMP或TMP环境变量目录中并启动执行
//[localhost:tests]# go run getPath.go
//2022/03/23 22:42:12 /root/GoWorld/src/sms/tests
//2022/03/23 22:42:12 /tmp/go-build332476320/b001/exe/getPath

//[localhost:tests]# go build getPath.go
//[localhost:tests]# ./getPath
//2022/03/23 22:42:43 /root/GoWorld/src/sms/tests
//2022/03/23 22:42:43 /root/GoWorld/src/sms/tests/getPath

func main() {
	str, err := os.Getwd()
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(str)

	str, err = os.Executable()
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(str)
}
