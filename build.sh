#!/bin/bash

go build -o sms main.go http.go rtmp.go serialize.go amf.go flv.go hls.go
echo "==========================================="
rm -rf sms.log
rm -rf streamlog
rm -rf hls
sleep 1s
./sms
