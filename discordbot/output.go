package discordbot

import (
	"bytes"
	"io"
	"os"
	"path"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
	"github.com/pkg/errors"
)

func (b *Bot) createOutput(id discord.MessageID, ext string) (*outputFile, error) {
	return createOutput(id, ext, b.cfg.OutputDir, b.cfg.OutputURL)
}

func (b *Bot) sendFile(ch discord.ChannelID, mid discord.MessageID,
	ext string, src io.Reader) error {
	buf := new(bytes.Buffer) // TODO sync.Pool of buffers?
	lr := &io.LimitedReader{R: src, N: 8000000}
	_, err := buf.ReadFrom(lr)
	if err != nil {
		return err
	}
	if lr.N > 0 {
		_, err := b.Ctx.SendMessageComplex(ch, api.SendMessageData{
			Files: []sendpart.File{{Name: mid.String() + "." + ext, Reader: buf}},
		})
		return err
	}
	out, err := b.createOutput(mid, ext)
	if err != nil {
		return err
	}
	defer out.Cleanup()
	_, err = buf.WriteTo(out.File)
	if err != nil {
		return err
	}
	_, err = io.Copy(out.File, src)
	if err != nil {
		return err
	}
	return out.Send(b.Ctx.Client, ch)
}

func createOutput(id discord.MessageID, ext,
	dir, baseurl string) (*outputFile, error) {
	f, err := os.CreateTemp("", id.String()+"."+ext)
	if err != nil {
		return nil, err
	}
	of := new(outputFile)
	of.baseurl = baseurl
	of.File = f
	of.name = id.String() + "." + ext
	if dir != "" {
		of.moveto = path.Join(dir, of.name)
	}
	return of, err
}

type outputFile struct {
	File    *os.File
	name    string
	moveto  string
	baseurl string
	moved   bool
	keep    bool
}

func (s *outputFile) Send(ctx *api.Client, id discord.ChannelID) error {
	f := s.File
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if stat.Size() <= 8000000 {
		_, err = ctx.SendMessageComplex(id, api.SendMessageData{
			Files: []sendpart.File{{Name: s.name, Reader: f}},
		})
		return err
	}
	if s.moveto == "" || s.baseurl == "" {
		return errors.New("file too large")
	}
	f.Close()
	err = os.Rename(f.Name(), s.moveto)
	if err != nil {
		return err
	}
	err = os.Chmod(s.moveto, 0644)
	if err != nil {
		return err
	}
	s.moved = true
	_, err = ctx.SendMessage(id, s.baseurl+s.name)
	if err == nil {
		s.keep = true
	}
	return err
}

func (s *outputFile) Cleanup() {
	s.File.Close()
	if s.keep {
		return
	}
	if s.moved {
		os.Remove(s.moveto)
	} else {
		os.Remove(s.File.Name())
	}
}
