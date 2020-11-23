package esammy

import (
	"github.com/diamondburned/arikawa/v2/discord"
	"github.com/pkg/errors"
)

type Media struct {
	URL    string
	Height int
	Width  int
	GIFV   bool
}

func (b *Bot) findMedia(m discord.Message) (*Media, error) {
	media := getMsgMedia(m)
	if media != nil {
		return media, nil
	}
	if m.ReferencedMessage != nil {
		media = getMsgMedia(*m.ReferencedMessage)
		if media != nil {
			return media, nil
		}
	}
	msgs, err := b.Ctx.Messages(m.ChannelID)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		media = getMsgMedia(m)
		if media != nil {
			return media, nil
		}
	}
	return nil, errors.New("no media found")
}
func getMsgMedia(m discord.Message) *Media {
	for _, at := range m.Attachments {
		if at.Height == 0 {
			continue
		}
		return &Media{
			URL:    at.Proxy,
			Height: int(at.Height),
			Width:  int(at.Width),
		}
	}
	for _, em := range m.Embeds {
		if em.Type == discord.VideoEmbed && em.Provider == nil {
			return &Media{
				URL:    em.Video.URL,
				Height: int(em.Video.Height),
				Width:  int(em.Video.Width),
			}
		}
		if em.Type == discord.ImageEmbed {
			return &Media{
				URL:    em.Thumbnail.Proxy,
				Height: int(em.Thumbnail.Height),
				Width:  int(em.Thumbnail.Width),
			}
		}
		if em.Type == discord.GIFVEmbed {
			return &Media{
				URL:    em.Video.URL,
				Height: int(em.Video.Height),
				Width:  int(em.Video.Width),
				GIFV:   true,
			}
		}
	}
	return nil
}
