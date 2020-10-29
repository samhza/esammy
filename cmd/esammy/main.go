package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"git.sr.ht/~emersion/go-scfg"
	"github.com/diamondburned/arikawa/bot"
	"git.sr.ht/~samhza/esammy"
	"git.sr.ht/~samhza/esammy/ff"
)

func init() {
	log.SetFlags(0)
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "esammy.config", "Path to load config from")
	flag.Parse()
	configFile, err := os.Open(configPath)
	if err != nil {
		log.Fatalln("error opening config file:", err)
	}
	config, err := scfg.Read(configFile)
	if err != nil {
		log.Fatalln("error reading config file:", err)
	}

	var token string
	var prefixes []string
	httpClient := &http.Client{Timeout: 30 * time.Second}
	ffThrottler := ff.NewThrottler(int64(runtime.GOMAXPROCS(-1) * 2))
	for _, d := range config {
		var err error
		switch d.Name {
		case "token":
			d.ParseParams(&token)
		case "prefix":
			prefixes = d.Params
		case "http-timeout":
			var timeoutStr string
			d.ParseParams(&timeoutStr)
			var timeout int
			timeout, err = strconv.Atoi(timeoutStr)
			if err != nil {
				break
			}
			httpClient.Timeout = time.Duration(timeout) * time.Millisecond
		case "max-ffmpeg-processes":
			var maxProcStr string
			d.ParseParams(&maxProcStr)
			var maxProc int64
			maxProc, err = strconv.ParseInt(maxProcStr, 10, 64)
			if err != nil {
				log.Fatalln("error parsing config file:", err)
			}
			ffThrottler = ff.NewThrottler(maxProc)
		}
	}

	esammy := esammy.New(httpClient, ffThrottler)
	wait, err := bot.Start(token, esammy, func(ctx *bot.Context) error {
		ctx.HasPrefix = bot.NewPrefix(prefixes...)
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}

	log.Println("Bot started")

	if err := wait(); err != nil {
		log.Fatalln("Gateway fatal error:", err)
	}
}
