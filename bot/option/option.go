package option

import (
	"errors"
	"fmt"
	"github.com/google/shlex"
	"regexp"
	"strconv"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json"
	"samhza.com/esammy/bot/plugin"
)

var _ plugin.Options = Default{}

type None struct{}

func (n None) Parse(s *state.State, context *plugin.Context) error {
	return nil
}

func (n None) ForDiscord() []discord.CommandOptionValue {
	return nil
}

type Default []Option

func (d Default) NRequired() int {
	for n, opt := range d {
		if !opt.Required {
			return n
		}
	}
	return len(d)
}

func (d Default) ForDiscord() []discord.CommandOptionValue {
	options := make([]discord.CommandOptionValue, len(d))
	for i, opt := range d {
		options[i] = opt.Type.ForDiscord(opt)
	}
	return options
}

func (d Default) Parse(s *state.State, ctx *plugin.Context) error {
	msg, ok := ctx.Source.(plugin.MessageSource)
	if !ok {
		dopts := ctx.InteractionOpts
		for _, dopt := range dopts {
			for _, opt := range d {
				if dopt.Name != opt.Name {
					continue
				}
				var err error
				ctx.Options[opt.Name], err = opt.Type.Parse(s, &ParseContext{
					Raw:     dopt.Value,
					Context: ctx,
				})
				if err != nil {
					return fmt.Errorf("failed to parse option \"%s\": %w", opt.Name, err)
				}
				break
			}
		}
		for _, opt := range d {
			if opt.Required {
				if _, ok := ctx.Options[opt.Name]; !ok {
					return fmt.Errorf("missing required option \"%s\"", opt.Name)
				}
			} else {
				break
			}
		}
		return nil
	}
	splat, err := shlex.Split(msg.Content[ctx.OptsIndex:])
	if err != nil {
		return fmt.Errorf("splitting arguments: %w", err)
	}
	fmt.Println(splat, d.NRequired())
	if len(splat) < d.NRequired() {
		return errors.New("not enough arguments")
	}
	for i, opt := range d {
		if i >= len(splat) {
			break
		}
		var err error
		ctx.Options[opt.Name], err = opt.Type.Parse(s, &ParseContext{
			RawString: splat[i],
			Context:   ctx,
		})
		if err != nil {
			return fmt.Errorf("failed to parse option \"%s\": %w", opt.Name, err)
		}
	}
	return nil
}

type Option struct {
	Name        string
	Description string
	Required    bool
	Type        Type
}

type Type interface {
	Parse(*state.State, *ParseContext) (any, error)
	ForDiscord(Option) discord.CommandOptionValue
}

type ParseContext struct {
	Raw       json.Raw
	RawString string
	*plugin.Context
}
type String struct{}

func (String) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.StringOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

func (String) Parse(s *state.State, ctx *ParseContext) (any, error) {
	if _, ok := ctx.Source.(plugin.InteractionSource); ok {
		var s string
		return s, ctx.Raw.UnmarshalTo(&s)
	}
	return ctx.RawString, nil
}

type Int64 struct{}

func (Int64) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.IntegerOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

func (Int64) Parse(s *state.State, ctx *ParseContext) (any, error) {
	if _, ok := ctx.Source.(plugin.InteractionSource); ok {
		var i int64
		return i, ctx.Raw.UnmarshalTo(&i)
	}
	return strconv.ParseInt(ctx.RawString, 10, 64)
}

type Bool struct{}

func (Bool) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.BooleanOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

func (Bool) Parse(s *state.State, ctx *ParseContext) (any, error) {
	if _, ok := ctx.Source.(plugin.InteractionSource); ok {
		var b bool
		return b, ctx.Raw.UnmarshalTo(&b)
	}
	return strconv.ParseBool(ctx.RawString)
}

type User struct{}

func (User) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.UserOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

var userMentionRegex = regexp.MustCompile(`<@!?(\d+)>`)

func (User) Parse(s *state.State, ctx *ParseContext) (any, error) {
	if src, ok := ctx.Source.(plugin.InteractionSource); ok {
		var rid discord.UserID
		if err := ctx.Raw.UnmarshalTo(&rid); err != nil {
			return nil, err
		}
		return src.Data.(*discord.CommandInteraction).Resolved.Users[rid], nil
	}
	var str string
	if matches := userMentionRegex.FindStringSubmatch(ctx.RawString); matches != nil {
		str = matches[1]
	} else {
		str = ctx.RawString
	}
	sf, err := discord.ParseSnowflake(str)
	if err != nil {
		return nil, fmt.Errorf("malformed user ID: %w", err)
	}
	user, err := s.User(discord.UserID(sf))
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return *user, nil
}

type Channel struct{}

func (Channel) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.ChannelOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

var channelMentionRegex = regexp.MustCompile(`<#(\d+)>`)

func (Channel) Parse(s *state.State, ctx *ParseContext) (any, error) {
	if src, ok := ctx.Source.(plugin.InteractionSource); ok {
		var rid discord.ChannelID
		if err := ctx.Raw.UnmarshalTo(&rid); err != nil {
			return nil, err
		}
		return src.Data.(*discord.CommandInteraction).Resolved.Channels[rid], nil
	}
	msg := ctx.Source.(plugin.MessageSource).Message
	if !msg.GuildID.IsValid() {
		return nil, errors.New("channel option can only be used in guilds")
	}
	var str string
	if matches := channelMentionRegex.FindStringSubmatch(ctx.RawString); matches != nil {
		str = matches[1]
	} else {
		str = ctx.RawString
	}
	sf, err := discord.ParseSnowflake(str)
	if err != nil {
		return nil, fmt.Errorf("malformed channel ID: %w", err)
	}
	channel, err := s.Channel(discord.ChannelID(sf))
	if err != nil {
		return nil, fmt.Errorf("channel not found: %w", err)
	}
	if channel.GuildID != msg.GuildID {
		return nil, errors.New("channel is not in guild")
	}
	return *channel, nil
}

type Mentionable struct{}

func (Mentionable) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.MentionableOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

type Float64 struct{}

func (Float64) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.NumberOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

type Role struct{}

func (Role) ForDiscord(opt Option) discord.CommandOptionValue {
	return &discord.RoleOption{
		OptionName:  opt.Name,
		Description: opt.Description,
		Required:    opt.Required,
	}
}

var roleMentionRegex = regexp.MustCompile(`<@&(\d+)>`)

func (Role) Parse(s *state.State, ctx *ParseContext) (any, error) {
	if src, ok := ctx.Source.(plugin.InteractionSource); ok {
		var rid discord.RoleID
		if err := ctx.Raw.UnmarshalTo(&rid); err != nil {
			return nil, err
		}
		return src.Data.(*discord.CommandInteraction).Resolved.Roles[rid], nil
	}
	msg := ctx.Source.(plugin.MessageSource).Message
	if !msg.GuildID.IsValid() {
		return nil, errors.New("role option can only be used in guilds")
	}
	var str string
	if matches := roleMentionRegex.FindStringSubmatch(ctx.RawString); matches != nil {
		str = matches[1]
	} else {
		str = ctx.RawString
	}
	sf, err := discord.ParseSnowflake(str)
	if err != nil {
		return nil, fmt.Errorf("malformed role ID: %w", err)
	}
	role, err := s.Role(msg.GuildID, discord.RoleID(sf))
	if err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}
	return *role, nil
}
