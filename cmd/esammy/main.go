package main

import (
	"flag"
	"log"

	"github.com/diamondburned/arikawa/bot"
	"samhza.com/esammy"
)

func main() {
	var token, prefix string
	flag.StringVar(&token, "token", "", "discord bot token")
	flag.StringVar(&prefix, "prefix", "!", "discord bot prefix")
	flag.Parse()
	if token == "" {
		log.Fatalln("No token provided")
	}
	esammy := esammy.New()
	wait, err := bot.Start(token, esammy, func(ctx *bot.Context) error {
		ctx.HasPrefix = bot.NewPrefix(prefix)
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
