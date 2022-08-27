package discordbot

import (
	"github.com/diamondburned/arikawa/v3/state"
	"mime"
	"path"
	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"samhza.com/esammy/tenor"
)

type Media struct {
	URL  string
	Type mediaType
}

type mediaType int

const (
	mediaImage mediaType = iota
	mediaVideo
	mediaGIFV
	mediaGIF
)

func (bot *Bot) findMsgMedia(s *state.State, m discord.Message) *Media {
	media := bot.getMsgMedia(m)
	if media != nil {
		return media
	}
	if m.Type == discord.InlinedReplyMessage && m.ReferencedMessage != nil {
		media = bot.getMsgMedia(*m.ReferencedMessage)
		if media != nil {
			return media
		}
	}
	msgs, err := s.Messages(m.ChannelID, 20)
	if err != nil {
		return nil
	}
	for _, m := range msgs {
		media = bot.getMsgMedia(m)
		if media != nil {
			return media
		}
	}
	return nil
}

func (bot *Bot) getMsgMedia(m discord.Message) *Media {
	for _, at := range m.Attachments {
		if at.Height == 0 {
			continue
		}
		ext := path.Ext(at.Proxy)
		m := &Media{
			URL:  at.Proxy,
			Type: mediaTypeByExt(ext),
		}
		return m
	}
	for _, em := range m.Embeds {
		if em.Type == discord.VideoEmbed && em.Provider == nil {
			return &Media{
				URL:  em.Video.URL,
				Type: mediaVideo,
			}
		}
		if em.Type == discord.ImageEmbed {
			m := &Media{
				URL: em.Thumbnail.Proxy,
			}
			m.Type = mediaTypeByExt(path.Ext(m.URL))
			return m
		}
		if em.Type == discord.GIFVEmbed {
			m := &Media{
				URL:  em.Video.URL,
				Type: mediaGIFV,
			}
			if gif := bot.gifURL(em.Video.URL); gif != "" {
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

func (bot *Bot) gifURL(gifvURL string) string {
	switch {
	case strings.HasPrefix(gifvURL, "https://tenor.com") && bot.tenor != nil:
		split := strings.Split(gifvURL, "-")
		id := split[len(split)-1]
		gifs, err := bot.tenor.GIFs([]string{id}, tenor.MediaFilterBasic, 1)
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
