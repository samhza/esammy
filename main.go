package main

import (
	"context"
	"flag"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"samhza.com/esammy/discordbot"

	"github.com/pelletier/go-toml"
)

type config struct {
	Token       string   `toml:"token"`
	HTTPTimeout int      `toml:"http-timeout" default:"30000"`
	Prefixes    []string `toml:"prefixes"`
	discordbot.Config
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
	s := state.New("Bot " + config.Token)
	s.AddIntents(gateway.IntentGuildMessages)
	bot, err := discordbot.New(httpClient, s, config.Config)
	if err != nil {
		log.Fatalln(err)
	}
	err = bot.Router.RegisterGuildCommands(1010039901410054214)
	if err != nil {
		log.Fatalln(err)
	}
	s.Open(context.Background())
	self, err := s.Me()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Bot logged in as %s#%s (%s)\n", self.Username, self.Discriminator, self.ID)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)
	<-sigs
	s.Close()
}
