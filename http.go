package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type Rsps struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func GetRsps(code int, msg string) []byte {
	r := Rsps{code, msg}
	d, err := json.Marshal(r)
	if err != nil {
		log.Println(err)
		return d
	}
	log.Println(string(d))
	return d
}

func GetVersion(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	s := fmt.Sprintf("%s %s", AppName, AppVersion)
	d := GetRsps(200, s)
	return d, nil
}

// GET http://www.domain.com/live/yuankang.flv
// GET http://www.domain.com/live/yuankang.m3u8
// GET http://www.domain.com/api/version
func HttpServer(w http.ResponseWriter, r *http.Request) {
	log.Println("====== new http request ======")
	log.Println(r.Proto, r.Method, r.URL, r.RemoteAddr, r.Host)

	var rsps []byte
	var err error

	if r.Method == "GET" {
		// HTTP/1.1 GET /api/version 127.0.0.1:63544
		if strings.Contains(r.URL.String(), "/api/version") {
			rsps, err = GetVersion(w, r)
			if err != nil {
				log.Println(err)
				goto ERR
			}
		} else if strings.Contains(r.URL.String(), ".flv") {
			GetFlv(w, r)
			return
		} else if strings.Contains(r.URL.String(), ".m3u8") {
			rsps, err = GetM3u8(w, r)
			if err != nil {
				log.Println(err)
				goto ERR
			}
			// safari地址栏输入播放地址  必须有这个 才能播放
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		} else if strings.Contains(r.URL.String(), ".ts") {
			rsps, err = GetTs(w, r)
			if err != nil {
				log.Println(err)
				goto ERR
			}
		} else {
			// HTTP/1.1 GET /favicon.ico
			err = fmt.Errorf("undefined GET request")
			goto ERR
		}
	} else if r.Method == "POST" {
		d, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Println(err)
			goto ERR
		}
		log.Println(string(d))

		err = fmt.Errorf("undefined POST request")
		goto ERR
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Content-length", strconv.Itoa(len(rsps)))
	w.Header().Set("Server", AppName)
	w.Write(rsps)
	return
ERR:
	//w.WriteHeader(500)
	rsps = GetRsps(500, err.Error())
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-length", strconv.Itoa(len(rsps)))
	w.Header().Set("Server", AppName)
	w.Write(rsps)
}
