package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"utils"

	"github.com/kardianos/service"
	"github.com/natefinch/lumberjack"
)

const (
	AppName    = "sms"
	AppVersion = "0.0.1"
	AppConf    = "sms.json"
)

var (
	h, v, d, u bool
	c          string
	conf       Config
	Publishers map[string]*Stream // App_PublishName
)

type Config struct {
	RtmpListen  string
	HttpListen  string
	HttpsListen string
	HttpsCrt    string
	HttpsKey    string
	HttpsUse    bool
	LogFile     string
	LogFileSize int
	LogFileNum  int
	LogSaveDay  int
}

func InitConf(file string) {
	s, err := utils.ReadAllFile(file)
	if err != nil {
		log.Fatalln(err)
	}

	err = json.Unmarshal([]byte(s), &conf)
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("%#v", conf)
}

func InitLog(file string) {
	log.SetFlags(log.LstdFlags | log.Lshortfile) // 前台打印
	return
	l := new(lumberjack.Logger)
	l.Filename = file
	l.MaxSize = conf.LogFileSize   //300 // megabytes
	l.MaxBackups = conf.LogFileNum //10
	l.MaxAge = conf.LogSaveDay     //15 //days

	log.SetOutput(l)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("==================================================")
	log.Println("== ", AppName, " version:", AppVersion)
	log.Println("== StartTime:", utils.GetYMDHMS())
	log.Println("== ByteOrder:", GetByteOrder())
	log.Println("==================================================")
	log.Println(h, v, d, u, c)
	log.Printf("%#v", conf)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		for {
			<-ch
			l.Rotate()
		}
	}()
}

/////////////////////////////////////////////////////////////////
// 守护进程 且 注册为系统服务(开机启动)
/////////////////////////////////////////////////////////////////
type program struct{}

func (p *program) run() {
	InitConf(c)
	InitLog(conf.LogFile)
	Publishers = make(map[string]*Stream)

	go RtmpServer()
	//go SipServer()

	http.HandleFunc("/", HttpServer)

	log.Println("start http listen on", conf.HttpListen)
	go func() {
		log.Fatal(http.ListenAndServe(conf.HttpListen, nil))
	}()

	if conf.HttpsUse {
		log.Println("start https listen on", conf.HttpsListen)
		go func() {
			log.Fatal(http.ListenAndServeTLS(conf.HttpsListen,
				conf.HttpsCrt, conf.HttpsKey, nil))
		}()
	}

	select {}
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	return nil
}

func main() {
	flag.BoolVar(&h, "h", false, "print help")
	flag.BoolVar(&v, "v", false, "print version")
	flag.BoolVar(&d, "d", false, "run in deamon")
	flag.BoolVar(&u, "u", false, "stop in deamon")
	flag.StringVar(&c, "c", AppConf, "config file")
	flag.Parse()
	//flag.Usage()
	log.Println(h, v, d, u, c)
	if h {
		flag.PrintDefaults()
		return
	}
	if v {
		log.Println(AppVersion)
		return
	}

	sc := new(service.Config)
	sc.Name = AppName
	sc.DisplayName = AppName
	sc.Description = AppName

	prg := new(program)
	s, err := service.New(prg, sc)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if u {
		err = service.Control(s, "stop")
		if err != nil {
			log.Println(err)
		} else {
			log.Println("service stopped")
		}
		err = service.Control(s, "uninstall")
		if err != nil {
			log.Println(err)
		} else {
			log.Println("service uninstalled")
		}
		return
	}

	if !d {
		prg.run()
		return
	}

	err = service.Control(s, "stop")
	if err != nil {
		log.Println(err)
	} else {
		log.Println("service stopped")
	}
	err = service.Control(s, "uninstall")
	if err != nil {
		log.Println(err)
	} else {
		log.Println("service uninstalled")
	}
	err = service.Control(s, "install")
	if err != nil {
		log.Println(err)
	} else {
		log.Println("service installed")
	}
	err = service.Control(s, "start")
	if err != nil {
		log.Println(err)
	} else {
		log.Println("service started")
	}
}
