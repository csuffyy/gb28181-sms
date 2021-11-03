package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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

func HttpServer(w http.ResponseWriter, r *http.Request) {
	log.Println("------>>> new http request")
	log.Println(r.Proto, r.Method, r.URL, r.RemoteAddr, r.Header["Upgrade"])

	rsps := GetRsps(200, "ok")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Content-length", strconv.Itoa(len(rsps)))
	w.Header().Set("Server", "StreamMediaServer")
	w.Write(rsps)
}
