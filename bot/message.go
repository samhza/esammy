package bot

import (
	"errors"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"samhza.com/esammy/bot/plugin"
)

/*
	func (r *Router) handleInteraction(ev *gateway.InteractionCreateEvent) {
		_, ok := ev.Data.(*discord.CommandInteraction)
		if !ok {
			return
		}
		user := ev.User
		if user == nil {
			user = &ev.Member.User
		}
		ctx := &plugin.Context{
			Source:    plugin.InteractionSource{ev.InteractionEvent},
			Member:    ev.Member,
			User:      user,
			GuildID:   ev.GuildID,
			ChannelID: ev.ChannelID,
			Replier:   interactionReplier{r.State, r.appID, ev.InteractionEvent},
			Provider:  &provider{r},
		}
		inv := r.applyMiddlewares()
		_, err := inv.Invoke(r.State, ctx)
		log.Println(err)
	}
*/
func (r *Router) handleMessage(m *gateway.MessageCreateEvent) {
	if m.Author.ID == r.self.ID {
		return
	}
	ctx := &plugin.Context{
		Source:    plugin.MessageSource{*m},
		Member:    m.Member,
		User:      m.Author,
		GuildID:   m.GuildID,
		ChannelID: m.ChannelID,
		ID:        discord.Snowflake(m.ID),
		Replier:   &messageReplier{State: r.State, m: m},
		Provider:  &provider{r},
		Options:   make(map[string]any),
	}
	inv := r.applyMiddlewares()
	_, err := inv.Invoke(r.State, ctx)
	if err != nil {
		ctx.Replier.Respond(plugin.ReplyData{
			Content: err.Error(),
		})
	}
}

func msgResp(data plugin.ReplyData, replyto discord.MessageID) api.SendMessageData {
	return api.SendMessageData{
		Content:         data.Content,
		Embeds:          data.Embeds,
		Components:      data.Components,
		AllowedMentions: data.AllowedMentions,
		Files:           data.Files,
		Reference: &discord.MessageReference{
			MessageID: replyto,
		},
	}
}

func msgEdit(data plugin.EditReplyData) api.EditMessageData {
	return api.EditMessageData{
		Content:         data.Content,
		Embeds:          data.Embeds,
		Components:      data.Components,
		AllowedMentions: data.AllowedMentions,
		Attachments:     data.Attachments,
		Files:           data.Files,
	}

}

type messageReplier struct {
	*state.State
	response discord.MessageID
	m        *gateway.MessageCreateEvent
	state    replierState
}

func (r *messageReplier) Defer() error {
	response, err := r.SendMessageReply(r.m.ChannelID, "Thinking...", r.m.Message.ID)
	if err != nil {
		return err
	}
	r.response = response.ID
	r.state = deferred
	return nil
}

func (r *messageReplier) Reply(d plugin.ReplyData) error {
	if r.state == deferred {
		_, err := r.EditResponse(plugin.EditReplyData{
			Content:         option.NewNullableString(d.Content),
			Embeds:          &d.Embeds,
			Components:      &d.Components,
			AllowedMentions: d.AllowedMentions,
			Files:           d.Files,
		})
		return err
	}
	if r.state == responded {
		_, err := r.Followup(d)
		return err
	}
	return r.Respond(d)
}

func (r *messageReplier) Respond(d plugin.ReplyData) error {
	if r.state == responded {
		return errors.New("replier: already responded")
	}
	if r.state == deferred {
		_, err := r.EditResponse(plugin.EditReplyData{
			Content:         option.NewNullableString(d.Content),
			Embeds:          &d.Embeds,
			Components:      &d.Components,
			AllowedMentions: d.AllowedMentions,
			Files:           d.Files,
		})
		return err
	}
	r.state = responded
	msg, err := r.SendMessageComplex(r.m.ChannelID, msgResp(d, r.m.ID))
	r.response = msg.ID
	return err
}

func (r *messageReplier) EditResponse(data plugin.EditReplyData) (*discord.Message, error) {
	return r.EditMessageComplex(r.m.ChannelID, r.response, msgEdit(data))
}

func (r *messageReplier) DeleteResponse() error {
	return r.DeleteMessage(r.m.ChannelID, r.response, "")
}

func (r *messageReplier) Followup(data plugin.ReplyData) (*discord.Message, error) {
	return r.SendMessageComplex(r.m.ChannelID, msgResp(data, r.m.ID))
}

func (r *messageReplier) EditFollowup(id discord.MessageID, data plugin.EditReplyData) (*discord.Message, error) {
	return r.EditMessageComplex(r.m.ChannelID, id, msgEdit(data))
}

func (r *messageReplier) DeleteFollowup(id discord.MessageID) error {
	return r.DeleteMessage(r.m.ChannelID, id, "")
}
