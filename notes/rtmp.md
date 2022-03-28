直播系统---从直播答题看SEI帧的原理和作用
https://blog.csdn.net/feeltouch/article/details/103333174
FFmpeg从入门到精通：SEI那些事
https://blog.csdn.net/vn9PLgZvnPs1522s82g/article/details/79320080

### rtmp推流结束过程
2022/03/23 23:54:53 rtmp.go:508: TimeDelta=0, Timestamp=5805, MsgLength=5, MsgTypeId=9, MsgStreamId=0
2022/03/23 23:54:53 rtmp.go:1058: This frame is AVC end of sequence

2022/03/23 23:54:53 rtmp.go:508: TimeDelta=0, Timestamp=0, MsgLength=32, MsgTypeId=20, MsgStreamId=0
2022/03/23 23:54:53 amf.go:66: Amf Unmarshal []interface {}{"FCUnpublish", 6, interface {}(nil), "cctv1"}
2022/03/23 23:54:53 amf.go:111: Untreated AmfCmd FCUnpublish

2022/03/23 23:54:53 rtmp.go:508: TimeDelta=0, Timestamp=0, MsgLength=34, MsgTypeId=20, MsgStreamId=0
2022/03/23 23:54:53 amf.go:111: Untreated AmfCmd deleteStream
