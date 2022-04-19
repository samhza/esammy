package discordbot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
	"github.com/minio/minio-go/v7"
)

// startWorking sends a "Working..." to inform the user that the bot is working
// on generating the output. The returned function must be called to delete the
// "Working..." message. The returned function may be called more than once, any
// calls after the first will be ignored.
func (b *Bot) startWorking(ch discord.ChannelID, m discord.MessageID) func() {
	done := make(chan struct{})
	timer := time.NewTimer(500 * time.Millisecond)
	go func() {
		var msg *discord.Message
		var err error
		select {
		case <-done:
			timer.Stop()
			return
		case <-timer.C:
			msg, err = b.Ctx.SendTextReply(ch, "Working...", m)
		}
		<-done
		if err != nil {
			return
		}
		b.Ctx.DeleteMessage(ch, msg.ID, "")
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
	}
}

func (b *Bot) createOutput(id discord.MessageID, ext string) (*outputFile, error) {
	f, err := os.CreateTemp(b.cfg.OutputDir, "*."+ext)
	if err != nil {
		return nil, err
	}
	of := new(outputFile)
	of.File = f
	of.name = id.String() + "." + ext
	of.bot = b
	return of, err
}

// sendFile sends the contents of a reader into a channel. See outputFile for
// more information.
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
			Reference: &discord.MessageReference{
				MessageID: mid,
			},
			Files: []sendpart.File{{Name: mid.String() + "." + ext, Reader: buf}},
		})
		return err
	}
	out, err := b.createOutput(mid, ext)
	if err != nil {
		return err
	}
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

// outputFile is a file that will be sent to Discord. If the file is small
// enough, it will be sent as a file attachment and deleted. If the file is too
// large, it will be moved to a file and sent as a link instead.
type outputFile struct {
	File *os.File
	name string
	bot  *Bot
}

func (s *outputFile) Send(ctx *api.Client, id discord.ChannelID) error {
	f := s.File
	defer func(name string) {
		f.Close()
		os.Remove(name)
	}(f.Name())
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
	var url string
	if s.bot.cfg.S3Endpoint != "" {
		f.Seek(0, 0)
		_, err = s.bot.s3.PutObject(context.Background(), s.bot.cfg.S3Bucket, s.name, f, stat.Size(), minio.PutObjectOptions{})
		if err != nil {
			return err
		}
		url = s.bot.cfg.OutputURL + s.name
	} else if s.bot.cfg.OutputDir != "" {
		f.Close()
		err = os.Rename(f.Name(), path.Join(s.bot.cfg.OutputDir+s.name))
		if err != nil {
			return err
		}
		url = s.bot.cfg.OutputURL + s.name
	} else {
		return errors.New("file too large and no S3/upload directory configured")
	}

	_, err = ctx.SendMessage(id, url)
	return err
}
