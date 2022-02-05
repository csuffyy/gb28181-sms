#!/bin/bash

go build -o sms main.go http.go rtmp.go serialize.go amf.go flv.go hls.go
rm -rf live_yuankang/
./sms
