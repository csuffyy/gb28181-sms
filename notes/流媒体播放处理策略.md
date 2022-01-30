### 流媒体播放处理策略

策略1，所有播放者，都在一个发送协程。实现简单，播放者网络会互相影响。
策略2，所有播放者，都在一个发送协程，每次发送都设置超时时间。超时时间设置多少合适？一个发送协程 最多容纳几个发送者？
策略3，每个播放者，一个独立发送协程。播放者不会互相影响，但是协程数太 多会降低整个软件的效率。
策略4，每个播放者，一个独立发送协程，所有播放者总数不能超过某个总数。
策略5，对于同一个流，可以有多个发送协程，每个发送协程内有n个播放者，n个播放者需要在1秒内发送完毕。

### 策略2详细说明

处理所有播放者 在一个发送协程，给每个播放者 发送数据时 都设定50ms超时。
给某个播放者发送数据时候，如果触发超时，对该播放者 超时计数+1，并接着给下个播放者发送数据。
如果某个播放者超时计数 连续累计大于3次，直接断开并销毁该播放者。
同时，建议播放器端，做2-5秒的数据缓存，虽然会增加直播的延时，但是能有效降低卡顿次数。

用这种方法，如果想把发送数据的延时控制在1秒内，一个发送协程里 最多只能有20个播放者
1000ms / 50ms = 20个播放者
1000ms / 100ms = 10个播放者，经过实际测试，发送延时 最好设置为100ms

### 验证方法

1 在A机器上运行 tcpServer ，在B机器上运行 tcpClient。2个程序的代码见下文，测试时需要修改ip地址。
2 B机器上 使用tc命令模拟网络延时和丢包，执行如下命令

```shell
tc qdisc show  // 显示现有规则
tc qdisc add dev ens33 root netem delay 60ms  //加入规则
tc qdisc del dev ens33 root netem delay 60ms  //删除规则
```

添加规则后，可以使用ping命令验证延时时间
tc的原理是，增加数据包cache队列，接收到的包线放入队列里，达到延时时间在送给操作系统，
所以只能对机器B 造成 接收超时，间接的对机器A造成接收超时，无法对机器A造成发送超时

3 修改 tcpServer 代码里的监听ip和发收数据的超时时间为50ms
4 修改 tcpClient 代码里的服务器ip
5 先运行 tcpServer 在运行 tcpClient，观察 tcpServer 是否会接收超时

### 用于验证网络延时的IP

221.181.38.25		  中国 新疆 乌鲁木齐 移动		 min/avg/max/stddev = 57.424/78.842/180.216/42.112 ms
36.101.208.75		  中国 海南 海口 电信				 min/avg/max/stddev = 50.436/56.684/61.001/3.834 ms
42.187.161.102		中国 天津 天津 腾讯云			 min/avg/max/stddev = 9.880/14.561/16.759/2.875 ms
110.242.68.4			中国 河北 保定 顺平县 联通	 min/avg/max/stddev = 12.440/15.659/18.998/2.948 ms
114.114.114.114	  中国 江苏 南京 电信				 min/avg/max/stddev = 12.739/18.917/44.690/9.159 ms
8.8.8.8					  美国 谷歌云							  min/avg/max/stddev = 41.898/46.223/49.054/2.907 ms

### tcpServer.go

```go
package main

import (
    "log"
    "net"
    "time"
)

func process(conn net.Conn) {
    defer conn.Close()
    for {
        var buf [128]byte
        conn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
        bt := time.Now().UnixNano() / 1e6
        aa := time.Now().Add(10*time.Millisecond).UnixNano() / 1e6
        n, err := conn.Read(buf[:])
        at := time.Now().UnixNano() / 1e6
        // log.Println(time.Now().String())
        log.Println(at, bt, aa, "ms", at-bt)
        if err != nil {
            log.Println(err)
            break
        }
        log.Println("read data len:", n)
        log.Println("read data content:", string(buf[:n]))

        // time.Sleep(time.Second * 1)
        // conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))

        // bt = time.Now().UnixNano() / 1e6
        // aa = time.Now().Add(100*time.Millisecond).UnixNano() / 1e6
        // log.Println(time.Now().String())
        n, err = conn.Write([]byte("hi troy"))
        if err != nil {
            log.Println(err)
            break
        }
        // at = time.Now().UnixNano() / 1e6
        // log.Println(time.Now().String())
        // log.Println("Write", at, bt, aa, "ms", at-bt)
        log.Println("write data len:", n)
        log.Println("write data content: hi troy")
    }
}

func main() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    log.Println("listen on 0.0.0.0:9090")

    listener, err := net.Listen("tcp", "192.168.0.108:9090")
    if err != nil {
        log.Printf("listen fail, err: %v\n", err)
        return
    }

    for {
        conn, err := listener.Accept()
        log.Println(conn.LocalAddr().Network(), conn.LocalAddr().String())
        log.Println(conn.RemoteAddr().Network(), conn.RemoteAddr().String())
        if err != nil {
            log.Printf("accept fail, err: %v\n", err)
            continue
        }
        go process(conn)
    }
}
```

### tcpClient.go

```go
package main

import (
    "log"
    "net"
    "syscall"

    "golang.org/x/sys/unix"
)

func tcpClient(str string) {
    netAddr := &net.TCPAddr{Port: 14100}
    log.Println(netAddr)

    d := net.Dialer{
        LocalAddr: netAddr,
        Control: func(network, address string, c syscall.RawConn) error {
            return c.Control(func(fd uintptr) {
                syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR, 1)
                syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
            })
        },
    }

    conn, err := d.Dial("tcp", "192.168.0.108:9090")
    if err != nil {
        log.Println(err)
        return
    }
    defer conn.Close()

    for {
        n, err := conn.Write([]byte(str))
        if err != nil {
            log.Println(err)
            break
        }
        log.Println("write data len:", n)
        log.Println("write data content:", str)
      
        // time.Sleep(10 * time.Hour)

        var buf [128]byte
        n, err = conn.Read(buf[:])
        if err != nil {
            log.Println(err)
            break
        }
        log.Println("read data len:", n)
        log.Println("read data content:", string(buf[:n]))
    }
}

func main() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)

    tcpClient("aaa")
    // go tcpClient("aaa")
    // log.Println("111")
    // time.Sleep(10 * time.Second)
    // log.Println("222")
    // go tcpClient("bbb")
    select {}
}
```

