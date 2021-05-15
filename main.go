package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"go.samhza.com/esammy/discordbot"

	"github.com/diamondburned/arikawa/v2/bot"
	"github.com/pelletier/go-toml"
)

func init() {
	log.SetFlags(0)
}

type config struct {
	Token       string   `toml:"token"`
	Tenor       string   `toml:"tenor"`
	HTTPTimeout int      `toml:"http-timeout" default:"30000"`
	Prefixes    []string `toml:"prefixes"`
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "esammy.toml", "Path to load config from")
	flag.Parse()
	configFile, err := os.Open(configPath)
	if err != nil {
		log.Fatalln("error opening config file:", err)
	}
	var config config
	dec := toml.NewDecoder(configFile)
	if err = dec.Decode(&config); err != nil {
		log.Fatalln("error reading config:", err)
	}

	httpClient := &http.Client{
		Timeout: time.Duration(config.HTTPTimeout) * time.Millisecond}

	dbot := discordbot.New(httpClient, config.Tenor)
	wait, err := bot.Start(config.Token, dbot, func(ctx *bot.Context) error {
		ctx.HasPrefix = bot.NewPrefix(config.Prefixes...)
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
