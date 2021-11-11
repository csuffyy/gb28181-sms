### 序列化是什么？
数据发送 之前要 序列化，数据接收 之后要 反序列化
序列化要考虑 使用 那种字节序

// 序列化 与 反序列化
### 字节序
大端 与 小端

### go有哪些数据类型，他们分别占几个字节
// 单字节的数据类型
// 多字节的数据类型

// 数据发送 之前要 序列化，数据接收 之后要 反序列化
// 序列化要考虑 使用 那种字节序

// 序列化 与 反序列化
// 字节序，大端 与 小端

// 单字节的数据类型
// 多字节的数据类型


// []byte 以大端字节序 转 uint32
// u32 := binary.BigEndian.Uint32(b)
// []byte 以小端字节序 转 uint32
// uint32 以大端字节序 转 []byte
// uint32 以小端字节序 转 []byte

```go
[ykMac:src]# grep -n "^type " $GOROOT/src/builtin/builtin.go
14:type bool bool
24:type uint8 uint8
28:type uint16 uint16
32:type uint32 uint32
36:type uint64 uint64
40:type int8 int8
44:type int16 int16
48:type int32 int32
52:type int64 int64
55:type float32 float32
58:type float64 float64
62:type complex64 complex64
66:type complex128 complex128
71:type string string
75:type int int
79:type uint uint
83:type uintptr uintptr
88:type byte = uint8
92:type rune = int32
106:type Type int
111:type Type1 int
115:type IntegerType int
119:type FloatType float32
123:type ComplexType complex64
260:type error interface {
```





/*
bool
string
int  int8  int16  int32  int64
uint uint8 uint16 uint32 uint64 uintptr
byte // alias for uint8
rune // alias for int32
     // represents a Unicode code point
float32 float64
complex64 complex128
*/

// []byte 以大端字节序 转 uint32
// []byte 以小端字节序 转 uint32
// uint32 以大端字节序 转 []byte
// uint32 以小端字节序 转 []byte

// 从 socket中 读取 x个字节, x[1-4], 以大端字节序 转 uint32
// 从 socket中 读取 x个字节, x[1-4], 以小端字节序 转 uint32
// uint32 以大端字节序 写入到 socket中
// uint32 以小端字节序 写入到 socket中
