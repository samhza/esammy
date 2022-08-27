package bot

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/state"
	"samhza.com/esammy/bot/plugin"
)

type Router struct {
	State           *state.State
	Middlewares     Middlewares
	PostMiddlewares Middlewares
	Prefixer        Prefixer
	cmds            map[string]plugin.Command
	cmdIDs          map[discord.CommandID]plugin.Command
	appID           discord.AppID
	self            *discord.User
}

type Prefixer func(s plugin.Source) string

type Middlewares []func(next plugin.Invoker) plugin.Invoker

type Options struct {
	State *state.State
	AppID discord.AppID
}

func New(opt Options) (*Router, error) {
	r := new(Router)
	r.State = opt.State
	var err error
	r.self, err = opt.State.Me()
	if err != nil {
		return nil, err
	}
	r.appID = opt.AppID
	r.cmds = make(map[string]plugin.Command)
	r.cmdIDs = make(map[discord.CommandID]plugin.Command)
	r.State.AddHandler(r.handleInteraction)
	r.State.AddHandler(r.handleMessage)
	r.AddMiddlewares(DisallowBots)
	r.AddMiddlewares(Settings)
	r.AddMiddlewares(CheckPrefix)
	r.AddPostMiddlewares(FindCommand)
	r.AddPostMiddlewares(ParseArgs)
	r.AddPostMiddlewares(InvokeCommand)
	return r, nil
}

func (r *Router) AddCommand(cmd plugin.Command) {
	r.cmds[cmd.GetName()] = cmd
}

func (r *Router) AddMiddlewares(m ...func(plugin.Invoker) plugin.Invoker) {
	r.Middlewares = append(r.Middlewares, m...)
}
func (r *Router) AddPostMiddlewares(m ...func(plugin.Invoker) plugin.Invoker) {
	r.PostMiddlewares = append(r.PostMiddlewares, m...)
}

func (r *Router) applyMiddlewares() plugin.Invoker {
	var inv plugin.Invoker = plugin.InvokerFunc(
		func(*state.State, *plugin.Context) (any, error) { return nil, nil },
	)
	middlewares := append(r.Middlewares, r.PostMiddlewares...)
	for i := len(middlewares) - 1; i >= 0; i-- {
		inv = middlewares[i](inv)
	}
	return inv
}

type provider struct {
	r *Router
}

func (p provider) FindCommand(name string) (plugin.Command, bool) {
	cmd, ok := p.r.cmds[name]
	return cmd, ok
}
