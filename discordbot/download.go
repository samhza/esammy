package discordbot

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func (b *Bot) Download(m *gateway.MessageCreateEvent, args bot.RawArguments) error {
	done := b.startWorking(m.ChannelID, m.ID)
	defer done()
	dir, err := os.MkdirTemp("", "esammy")
	defer os.RemoveAll(dir)
	cmd := exec.Command(
		"yt-dlp",
		"--no-playlist",
		"--max-filesize", "500m",
		"--print", "filename",
		"--no-simulate",
		"-S", "mp4",
		"--match-filter", "duration <=? 600 & !was_live & !is_live",
		"--output", filepath.Join(dir, "%(title)s %(id)s.%(ext)s"),
		"--",
		string(args))
	stderr := strings.Builder{}
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		var exitError *exec.ExitError
		if err == nil || errors.As(err, &exitError) {
			return fmt.Errorf(string(stderr.String()))
		}
		return err
	}
	outfile := string(out[:len(out)-1])
	_, basename := filepath.Split(outfile)
	ext := filepath.Ext(outfile)
	f, err := os.Open(outfile)
	if err != nil {
		return err
	}
	of := new(outputFile)
	of.File = f
	of.mid = m.ID
	of.name = basename[:len(basename)-len(ext)]
	of.ext = ext
	of.bot = b
	return of.Send(b.Ctx.Client, m.ChannelID)
}
