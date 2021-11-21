package main

import (
	"log"
	"os"
)

func main() {
	f, err := os.Create("aaa.log")
	if err != nil {
		log.Println(err)
		return
	}

	var l *log.Logger
	l = log.New(f, "", log.LstdFlags|log.Lshortfile)
	INFO := func(s string) {
		//l.Output(1, s)
		l.Output(2, s)
	}
	l.Println("111")

	// 文件打开状态下 也可以重命名
	//func Mkdir(name string, perm FileMode) error {
	err = os.MkdirAll("live/test", 0755)
	if err != nil {
		log.Println(err)
		return
	}

	err = os.Rename("aaa.log", "live/test/bbb.log")
	if err != nil {
		log.Println(err)
		return
	}

	INFO("222")
	l.Println("333")
}
