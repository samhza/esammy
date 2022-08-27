package plugin

import (
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
)

// Context is the context provided to a command being invoked
type Context struct {
	Member          *discord.Member
	User            discord.User
	ChannelID       discord.ChannelID
	GuildID         discord.GuildID
	ID              discord.Snowflake
	RootCommand     Command
	Command         Command
	InteractionOpts discord.CommandInteractionOptions
	Prefixes        []string
	Options         map[string]any
	Source          Source
	InvokeIndex     int
	CommandIndex    int
	OptsIndex       int
	Replier         Replier
	Provider
}

type Provider interface {
	FindCommand(name string) (Command, bool)
}

func (c *Context) IsInteraction() bool {
	return c.Source.Type() == SourceInteraction
}

type Replier interface {
	Defer() error
	Reply(ReplyData) error
	Respond(ReplyData) error
	EditResponse(EditReplyData) (*discord.Message, error)
	DeleteResponse() error
	Followup(ReplyData) (*discord.Message, error)
	EditFollowup(discord.MessageID, EditReplyData) (*discord.Message, error)
	DeleteFollowup(discord.MessageID) error
}

type Source interface {
	Type() SourceType
}

type SourceType int

const (
	SourceMessage SourceType = iota
	SourceInteraction
)

type MessageSource struct {
	gateway.MessageCreateEvent
}

func (MessageSource) Type() SourceType {
	return SourceMessage
}

type InteractionSource struct {
	discord.InteractionEvent
}

func (InteractionSource) Type() SourceType {
	return SourceInteraction
}

type InvokerFunc func(*state.State, *Context) (any, error)

func (f InvokerFunc) Invoke(state *state.State, ctx *Context) (any, error) {
	return f(state, ctx)
}

type Invoker interface {
	Invoke(*state.State, *Context) (any, error)
}

type Command interface {
	Invoker
	CommandMeta
}

type CommandGroup interface {
	GetName() string
	GetDescription() string
	GetCommands() []Command
}

type CommandMeta interface {
	GetName() string
	GetDescription() string
	GetGroups() []CommandGroup
	GetSubcommands() []Command
	GetOptions() Options
}

type Options interface {
	Parse(*state.State, *Context) error
	ForDiscord() []discord.CommandOptionValue
}

type ParseContext struct {
	State  *state.State
	Value  any
	Source Source
}

type ReplyData struct {
	Content         string                      `json:"content,omitempty"`
	Embeds          []discord.Embed             `json:"embeds,omitempty"`
	Components      discord.ContainerComponents `json:"components,omitempty"`
	AllowedMentions *api.AllowedMentions        `json:"allowed_mentions,omitempty"`
	Flags           discord.MessageFlags        `json:"flags,omitempty"`
	Files           []sendpart.File             `json:"-"`
}

type EditReplyData struct {
	Content         option.NullableString        `json:"content,omitempty"`
	Embeds          *[]discord.Embed             `json:"embeds,omitempty"`
	Components      *discord.ContainerComponents `json:"components,omitempty"`
	AllowedMentions *api.AllowedMentions         `json:"allowed_mentions,omitempty"`
	Attachments     *[]discord.Attachment        `json:"attachments,omitempty"`
	Files           []sendpart.File              `json:"-"`
}
