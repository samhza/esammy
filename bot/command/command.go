package command

import (
	"fmt"
	"samhza.com/esammy/bot/plugin"
)

var _ plugin.Command = Command{}

func New(f plugin.InvokerFunc, name, description string, options plugin.Options) plugin.Command {
	return Command{
		f,
		Meta{
			Name:        name,
			Description: description,
			Options:     options,
		},
	}
}

type Command struct {
	plugin.InvokerFunc
	Meta
}

type Group struct {
	Name        string
	Description string
	Commands    []plugin.Command
}

func (g Group) GetName() string {
	return g.Name
}

func (g Group) GetDescription() string {
	return g.Description
}

func (g Group) GetCommands() []plugin.Command {
	return g.Commands
}

type Meta struct {
	Name        string
	Description string
	Groups      []plugin.CommandGroup
	Subcommands []plugin.Command
	Options     plugin.Options
}

func (m Meta) GetName() string {
	fmt.Println(m.Name, "GRHA")
	return m.Name
}

func (m Meta) GetDescription() string {
	return m.Description
}

func (m Meta) GetGroups() []plugin.CommandGroup {
	return m.Groups
}

func (m Meta) GetSubcommands() []plugin.Command {
	return m.Subcommands
}

func (m Meta) GetOptions() plugin.Options {
	return m.Options
}
