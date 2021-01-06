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
	"git.sr.ht/~samhza/esammy"
	"git.sr.ht/~samhza/esammy/ff"
	"github.com/diamondburned/arikawa/v2/bot"
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

	var token, tenor string
	var prefixes []string
	httpClient := &http.Client{Timeout: 30 * time.Second}
	ffThrottler := ff.NewThrottler(int64(runtime.GOMAXPROCS(-1) * 2))
	for _, d := range config {
		var err error
		switch d.Name {
		case "token":
			err = d.ParseParams(&token)
		case "tenor":
			err = d.ParseParams(&tenor)
		case "prefix":
			prefixes = d.Params
		case "http-timeout":
			var timeoutStr string
			err = d.ParseParams(&timeoutStr)
			var timeout int
			timeout, err = strconv.Atoi(timeoutStr)
			if err != nil {
				break
			}
			httpClient.Timeout = time.Duration(timeout) * time.Millisecond
		case "max-ffmpeg-processes":
			var maxProcStr string
			err = d.ParseParams(&maxProcStr)
			var maxProc int64
			maxProc, err = strconv.ParseInt(maxProcStr, 10, 64)
			if err != nil {
				log.Fatalln("error parsing config file:", err)
			}
			ffThrottler = ff.NewThrottler(maxProc)
		}
		if err != nil {
			log.Fatalf("failed to load config: %v\n", err)
		}
	}

	esammy := esammy.New(httpClient, tenor, ffThrottler)
	wait, err := bot.Start(token, esammy, func(ctx *bot.Context) error {
		ctx.HasPrefix = bot.NewPrefix(prefixes...)
		ctx.SilentUnknown.Command = true
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
