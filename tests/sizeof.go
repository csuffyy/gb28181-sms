package main

import (
	"log"
	"unsafe"
)

/*
最终的运算结果是：3, 16    3很容易理解，但是16从何而来呢？
unsafe.Sizeof() 和C语言的sizeof一样, 返回数据占用空间的字节数
而string在Go中并不是直存类型，它是一个结构体类型：
/usr/local/go/src/reflect/value.go:2324:type StringHeader struct {
type StringHeader struct {
	Data uintptr
	Len  int
}
在64位系统上uintptr和int都是8字节，加起来就16了。
*/

func main() {
	var s string = "abc"
	a := len(s)
	b := unsafe.Sizeof(s)
	log.Printf("data type is %T, len=%d, sizeof=%d", s, a, b)
}
