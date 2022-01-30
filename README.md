### 需要解决的问题
1 播放发送数据，要修改fmt值和修改协议头
2 多个播放在一个协程里，如何避免互相阻塞
golang网络通信超时设置
https://www.cnblogs.com/lanyangsh/p/10852755.html

# sms  
Stream Media Server  

### 编译命令  
go build -o sms main.go http.go rtmp.go  

### 支持的协议  
rtmp -> sms -> rtmp  

### rtmp参考资料
https://blog.csdn.net/lightfish_zhang/article/details/88681828
https://www.cnblogs.com/jimodetiantang/p/8974075.html
