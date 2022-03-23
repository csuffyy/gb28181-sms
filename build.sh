#!/bin/bash

go build -o sms main.go http.go rtmp.go serialize.go amf.go flv.go hls.go
echo "==========================================="
rm -rf live_cctv1
./sms
