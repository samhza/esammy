package esammy

import (
	"mime"
	"path"
	"strings"

	"github.com/diamondburned/arikawa/v2/discord"
	"github.com/pkg/errors"
	"go.samhza.com/esammy/tenor"
)

type Media struct {
	URL    string
	Height int
	Width  int
	Type   mediaType
}

type mediaType int

const (
	mediaImage mediaType = iota
	mediaVideo
	mediaGIFV
	mediaGIF
)

func (b *Bot) findMedia(m discord.Message) (*Media, error) {
	media := b.getMsgMedia(m)
	if media != nil {
		return media, nil
	}
	if m.Type == discord.InlinedReplyMessage && m.ReferencedMessage != nil {
		media = b.getMsgMedia(*m.ReferencedMessage)
		if media != nil {
			return media, nil
		}
	}
	msgs, err := b.Ctx.Messages(m.ChannelID)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		media = b.getMsgMedia(m)
		if media != nil {
			return media, nil
		}
	}
	return nil, errors.New("no media found")
}

func (b *Bot) getMsgMedia(m discord.Message) *Media {
	for _, at := range m.Attachments {
		if at.Height == 0 {
			continue
		}
		ext := path.Ext(at.Proxy)
		m := &Media{
			URL:    at.Proxy,
			Height: int(at.Height),
			Width:  int(at.Width),
			Type:   mediaTypeByExt(ext),
		}
		return m
	}
	for _, em := range m.Embeds {
		if em.Type == discord.VideoEmbed && em.Provider == nil {
			return &Media{
				URL:    em.Video.URL,
				Height: int(em.Video.Height),
				Width:  int(em.Video.Width),
				Type:   mediaVideo,
			}
		}
		if em.Type == discord.ImageEmbed {
			m := &Media{
				URL:    em.Thumbnail.Proxy,
				Height: int(em.Thumbnail.Height),
				Width:  int(em.Thumbnail.Width),
			}
			m.Type = mediaTypeByExt(path.Ext(m.URL))
			return m
		}
		if em.Type == discord.GIFVEmbed {
			m := &Media{
				Height: int(em.Video.Height),
				Width:  int(em.Video.Width),
				URL:    em.Video.URL,
				Type:   mediaGIFV,
			}
			if gif := b.gifURL(em.Video.URL); gif != "" {
				m.URL = gif
				m.Type = mediaGIF
			}
			return m
		}
	}
	return nil
}

func mediaTypeByExt(ext string) mediaType {
	mime := mime.TypeByExtension(ext)
	switch {
	case mime == "image/gif":
		return mediaGIF
	case strings.HasPrefix(mime, "video/"):
		return mediaVideo
	case strings.HasPrefix(mime, "image/"):
		return mediaImage
	}
	return mediaImage
}

func (b *Bot) gifURL(gifvURL string) string {
	switch {
	case strings.HasPrefix(gifvURL, "https://tenor.com") && b.tenor != nil:
		split := strings.Split(gifvURL, "-")
		id := split[len(split)-1]
		gifs, err := b.tenor.GIFs([]string{id}, tenor.MediaFilterBasic, 1)
		if err != nil || len(gifs) < 1 {
			break
		}
		return gifs[0].Media[0][tenor.FormatMediumGIF].URL
	case strings.HasPrefix(gifvURL, "https://giphy.com"):
		split := strings.Split(gifvURL, "-")
		id := split[len(split)-1]
		return "https://media0.giphy.com/media/" + id + "/giphy.gif"
	case strings.HasPrefix(gifvURL, "https://imgur.com"):
		split := strings.Split(gifvURL, "/")
		id := split[len(split)-1]
		return "https://i.imgur.com/" + id + ".gif"
	}
	return ""
}
