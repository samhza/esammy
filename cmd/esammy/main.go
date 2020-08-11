package main

import (
	"log"
	"os"

	"github.com/diamondburned/arikawa/bot"
	"github.com/erebid/esammy"
)

func main() {
	esammy := esammy.New()
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatalln("No token provided")
	}
	wait, err := bot.Start(token, esammy, func(ctx *bot.Context) error {
		ctx.HasPrefix = bot.NewPrefix("!")
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
