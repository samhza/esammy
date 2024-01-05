package discordbot

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/gateway"
)

//go:embed download.py
var downloadScript []byte

func (b *Bot) Download(m *gateway.MessageCreateEvent, args bot.RawArguments) error {
	done := b.startWorking(m.ChannelID, m.ID)
	defer done()
	cmd := exec.Command("python3", "-", string(args))
	stderr := strings.Builder{}
	cmd.Stdin = bytes.NewReader(downloadScript)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return fmt.Errorf("exit status %d: %s",
				exitError.ExitCode(), string(stderr.String()))
		}
		return err
	}
	outfile := string(out[:len(out)-1])
	ext := filepath.Ext(outfile)
	f, err := os.Open(outfile)
	if err != nil {
		return err
	}
	of := new(outputFile)
	of.File = f
	of.name = m.ID.String() + "." + ext
	of.bot = b
	return of.Send(b.Ctx.Client, m.ChannelID)
}
