package bot

import (
	"errors"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"log"
	"samhza.com/esammy/bot/plugin"
)

func (r *Router) RegisterCommands() error {
	return r.registerCommands(0)
}

func (r *Router) RegisterGuildCommands(gid discord.GuildID) error {
	return r.registerCommands(gid)
}

func (r *Router) registerCommands(gid discord.GuildID) error {
	cmds, err := r.appCommands()
	if err != nil {
		return err
	}
	var dcmds []discord.Command
	if gid == 0 {
		dcmds, err = r.State.BulkOverwriteCommands(r.appID, cmds)
	} else {
		dcmds, err = r.State.BulkOverwriteGuildCommands(r.appID, gid, cmds)
	}
	if err != nil {
		return err
	}
	for _, cmd := range dcmds {
		r.cmdIDs[cmd.ID] = r.cmds[cmd.Name]
	}
	return nil
}

func (r *Router) appCommands() ([]api.CreateCommandData, error) {
	cmds := make([]api.CreateCommandData, len(r.cmds))
	i := 0
	for _, cmd := range r.cmds {
		cmds[i] = cmdForDiscord(cmd)
		i++
	}
	return cmds, nil
}

func cmdForDiscord(cmd plugin.Command) api.CreateCommandData {
	dcmd := api.CreateCommandData{
		Name:        cmd.GetName(),
		Description: cmd.GetDescription(),
	}
	if opts := cmd.GetOptions(); opts != nil {
		opts := cmd.GetOptions().ForDiscord()
		dopts := make(discord.CommandOptions, len(opts))
		for i, opt := range opts {
			dopts[i] = opt
		}
		dcmd.Options = dopts
	} else {
		grps := cmd.GetGroups()
		subs := cmd.GetSubcommands()
		dcmd.Options = make(discord.CommandOptions, len(grps)+len(subs))
		i := 0
		for _, grp := range cmd.GetGroups() {
			subs := grp.GetCommands()
			dsubs := make([]*discord.SubcommandOption, len(subs))
			for j, sub := range subs {
				sub := subForDiscord(sub)
				dsubs[j] = &sub
			}
			dcmd.Options[i] = &discord.SubcommandGroupOption{
				OptionName:  grp.GetName(),
				Description: grp.GetDescription(),
				Subcommands: dsubs,
			}
			i++
		}
		for _, sub := range cmd.GetSubcommands() {
			sub := subForDiscord(sub)
			dcmd.Options[i] = &sub
			i++
		}
	}
	return dcmd
}

func subForDiscord(cmd plugin.Command) discord.SubcommandOption {
	opts := cmd.GetOptions().ForDiscord()
	dopts := make([]discord.CommandOptionValue, len(opts))
	for i, opt := range opts {
		dopts[i] = opt
	}
	return discord.SubcommandOption{
		OptionName:  cmd.GetName(),
		Description: cmd.GetDescription(),
		Options:     dopts,
	}
}

func (r *Router) handleInteraction(ev *gateway.InteractionCreateEvent) {
	_, ok := ev.Data.(*discord.CommandInteraction)
	if !ok {
		return
	}
	var user discord.User
	if ev.User != nil {
		user = *ev.User
	} else {
		user = ev.Member.User
	}
	ctx := &plugin.Context{
		Source:    plugin.InteractionSource{ev.InteractionEvent},
		Member:    ev.Member,
		User:      user,
		GuildID:   ev.GuildID,
		ChannelID: ev.ChannelID,
		ID:        discord.Snowflake(ev.ID),
		Replier: &interactionReplier{
			State: r.State,
			appID: r.appID,
			e:     ev.InteractionEvent},
		Provider: &provider{r},
		Options:  make(map[string]any),
	}
	inv := r.applyMiddlewares()
	_, err := inv.Invoke(r.State, ctx)
	if err != nil {
		log.Println(err)
		ctx.Replier.Respond(plugin.ReplyData{
			Content: err.Error(),
		})
	}
}

type interactionReplier struct {
	*state.State
	appID discord.AppID
	e     discord.InteractionEvent
	state replierState
}

type replierState int

const (
	unresponded replierState = iota
	deferred
	responded
)

func (r *interactionReplier) Defer() error {
	err := r.RespondInteraction(r.e.ID, r.e.Token, api.InteractionResponse{
		Type: api.DeferredMessageInteractionWithSource,
	})
	if err != nil {
		return err
	}
	r.state = deferred
	return nil
}

func (r *interactionReplier) Reply(d plugin.ReplyData) error {
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

func (r *interactionReplier) Respond(d plugin.ReplyData) error {
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
	return r.RespondInteraction(r.e.ID, r.e.Token, api.InteractionResponse{
		Type: api.MessageInteractionWithSource,
		Data: intResp(d),
	})
}

func (r *interactionReplier) EditResponse(data plugin.EditReplyData) (*discord.Message, error) {
	r.state = responded
	return r.EditInteractionResponse(r.appID, r.e.Token, *intEdit(data))
}

func (r *interactionReplier) Followup(data plugin.ReplyData) (*discord.Message, error) {
	return r.FollowUpInteraction(r.appID, r.e.Token, *intResp(data))
}

func (r *interactionReplier) EditFollowup(mid discord.MessageID, data plugin.EditReplyData) (*discord.Message, error) {
	return r.EditInteractionFollowup(r.appID, mid, r.e.Token, *intEdit(data))
}

func (r *interactionReplier) DeleteResponse() error {
	return r.DeleteInteractionResponse(r.appID, r.e.Token)
}

func (r *interactionReplier) DeleteFollowup(mid discord.MessageID) error {
	return r.DeleteInteractionFollowup(r.appID, mid, r.e.Token)
}

func intResp(data plugin.ReplyData) *api.InteractionResponseData {
	return &api.InteractionResponseData{
		Content:         option.NewNullableString(data.Content),
		Embeds:          &data.Embeds,
		Components:      &data.Components,
		AllowedMentions: data.AllowedMentions,
		Flags:           data.Flags,
		Files:           data.Files,
	}
}

func intEdit(data plugin.EditReplyData) *api.EditInteractionResponseData {
	return &api.EditInteractionResponseData{
		Content:         data.Content,
		Embeds:          data.Embeds,
		Components:      data.Components,
		AllowedMentions: data.AllowedMentions,
		Attachments:     data.Attachments,
		Files:           data.Files,
	}

}
