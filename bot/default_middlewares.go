package bot

import (
	"errors"
	"fmt"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/state"
	"log"
	"regexp"
	"samhza.com/esammy/bot/plugin"
	"strings"
	"sync"
)

func DisallowBots(next plugin.Invoker) plugin.Invoker {
	fn := func(s *state.State, ctx *plugin.Context) (interface{}, error) {
		if ctx.Source.Type() == plugin.SourceInteraction ||
			!ctx.Source.(plugin.MessageSource).Author.Bot {
			return next.Invoke(s, ctx)
		}
		return nil, nil
	}
	return plugin.InvokerFunc(fn)
}

func Settings(next plugin.Invoker) plugin.Invoker {
	prefixes := []string{"!!"}
	fn := func(s *state.State, ctx *plugin.Context) (interface{}, error) {
		ctx.Prefixes = prefixes
		return next.Invoke(s, ctx)
	}
	return plugin.InvokerFunc(fn)
}

func CheckPrefix(next plugin.Invoker) plugin.Invoker {
	var selfMentionRegexp *regexp.Regexp
	var once sync.Once
	fn := func(s *state.State, ctx *plugin.Context) (v any, err error) {
		once.Do(func() {
			var me *discord.User
			me, err = s.Me()
			if err != nil {
				return
			}
			selfMentionRegexp = regexp.MustCompile("^<@!?" + me.ID.String() + ">")
		})
		if ctx.IsInteraction() {
			return next.Invoke(s, ctx)
		}
		fmt.Println("lets check prefixes")
		msg := ctx.Source.(plugin.MessageSource)

		indexes := selfMentionRegexp.FindStringIndex(msg.Content)
		if indexes != nil { // invoked by mention
			ctx.InvokeIndex = len(msg.Content) - len(strings.TrimLeft(msg.Content[indexes[1]:], " "))
			return next.Invoke(s, ctx)
		}

		for _, p := range ctx.Prefixes {
			if strings.HasPrefix(msg.Content, p) {
				ctx.InvokeIndex = len(msg.Content) - len(strings.TrimLeft(msg.Content[len(p):], " "))
				return next.Invoke(s, ctx)
			}
		}

		// prefixes aren't required in direct messages, so DMs always "match"
		if ctx.GuildID == 0 {
			return next.Invoke(s, ctx)
		}

		return nil, nil
	}

	return plugin.InvokerFunc(fn)
}

func FindCommand(next plugin.Invoker) plugin.Invoker {
	fn := func(s *state.State, ctx *plugin.Context) (v any, err error) {
		if i, ok := ctx.Source.(plugin.InteractionSource); ok {
			ctx.RootCommand, ok = ctx.FindCommand(i.Data.(*discord.CommandInteraction).Name)
			if !ok {
				return nil, nil
			}
			return next.Invoke(s, ctx)
		}
		msg := ctx.Source.(plugin.MessageSource)
		i := strings.Index(msg.Content[ctx.InvokeIndex:], " ")
		if i == -1 {
			i = len(msg.Content) - ctx.InvokeIndex
		}
		var ok bool
		ctx.RootCommand, ok = ctx.FindCommand(msg.Content[ctx.InvokeIndex : ctx.InvokeIndex+i])
		if !ok {
			return nil, nil
		}
		ctx.CommandIndex = len(msg.Content) - len(strings.TrimLeft(msg.Content[ctx.InvokeIndex+i:], " "))
		return next.Invoke(s, ctx)
	}
	return plugin.InvokerFunc(fn)
}

func ParseArgs(next plugin.Invoker) plugin.Invoker {
	fn := func(s *state.State, ctx *plugin.Context) (v any, err error) {
		fmt.Println("PARS COMMAND")
		if ev, ok := ctx.Source.(plugin.InteractionSource); ok {
			data := ev.Data.(*discord.CommandInteraction)
			options := data.Options
			if len(options) > 0 {
				if options[0].Type == discord.SubcommandGroupOptionType {
					grps := ctx.RootCommand.GetGroups()
					for _, grp := range grps {
						if grp.GetName() != options[0].Name {
							continue
						}
						subs := grp.GetCommands()
						for _, sub := range subs {
							if sub.GetName() != options[0].Options[0].Name {
								continue
							}
							ctx.Command = sub
							break
						}
					}
					options = options[0].Options[0].Options
				}
				if options[0].Type == discord.SubcommandOptionType {
					subs := ctx.RootCommand.GetSubcommands()
					for _, sub := range subs {
						if sub.GetName() != options[0].Name {
							continue
						}
						ctx.Command = sub
						break
					}
					options = options[0].Options
				}
			}
			if ctx.Command == nil {
				ctx.Command = ctx.RootCommand
			}
			ctx.InteractionOpts = options
			if opts := ctx.Command.GetOptions(); opts != nil {
				err = opts.Parse(s, ctx)
				fmt.Println("error parsing command args:", err)
			}
			return next.Invoke(s, ctx)
		}
		var (
			msg  = ctx.Source.(plugin.MessageSource)
			grps = ctx.RootCommand.GetGroups()
			subs = ctx.RootCommand.GetSubcommands()
		)
		if len(grps) > 0 || len(subs) > 0 {
			i := strings.Index(msg.Content[ctx.CommandIndex:], " ")
			if i == -1 {
				i = len(msg.Content) - ctx.CommandIndex
			}
			var group plugin.CommandGroup
			cmdIndex := ctx.CommandIndex
			for _, grp := range grps {
				if grp.GetName() != msg.Content[ctx.CommandIndex:ctx.CommandIndex+i] {
					continue
				}
				group = grp
				cmdIndex = len(msg.Content) - len(strings.TrimLeft(msg.Content[ctx.CommandIndex+i:], " "))
				i = strings.Index(msg.Content[cmdIndex:], " ")
				if i == -1 {
					return nil, errors.New("missing subcommand b")
				}
				cmdIndex = len(msg.Content) - len(strings.TrimLeft(msg.Content[cmdIndex+i:], " "))
				break
			}
			if group != nil {
				subs = group.GetCommands()
			}
			for _, sub := range subs {
				log.Println("sub:", sub.GetName(), "cmd:", msg.Content[cmdIndex:])
				if sub.GetName() != msg.Content[cmdIndex:cmdIndex+i] {
					continue
				}
				ctx.Command = sub
				ctx.OptsIndex = len(msg.Content) - len(strings.TrimLeft(msg.Content[cmdIndex+i:], " "))
				break
			}
			if ctx.Command == nil {
				return nil, errors.New("subcommand not found")
			}
		} else {
			ctx.Command = ctx.RootCommand
			ctx.OptsIndex = ctx.CommandIndex
		}
		opts := ctx.Command.GetOptions()
		if opts == nil {
			return next.Invoke(s, ctx)
		}
		err = opts.Parse(s, ctx)
		if err != nil {
			return nil, err
		}
		return next.Invoke(s, ctx)
	}
	return plugin.InvokerFunc(fn)
}

func InvokeCommand(next plugin.Invoker) plugin.Invoker {
	fn := func(s *state.State, ctx *plugin.Context) (v any, err error) {
		out, err := ctx.Command.Invoke(s, ctx)
		if err != nil {
			return nil, ctx.Replier.Respond(plugin.ReplyData{
				Content: err.Error(),
			})
		}
		if out != nil {
			err = ctx.Replier.Respond(plugin.ReplyData{
				Content: fmt.Sprint(out),
			})
			if err != nil {
				return nil, err
			}
		}
		return next.Invoke(s, ctx)
	}
	return plugin.InvokerFunc(fn)
}
