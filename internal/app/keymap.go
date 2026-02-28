package app

import (
	"github.com/charmbracelet/bubbles/key"
)

type keyMap struct {
	move       key.Binding
	focus      key.Binding
	open       key.Binding
	search     key.Binding
	back       key.Binding
	help       key.Binding
	quit       key.Binding
	detailPane key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		move: key.NewBinding(
			key.WithKeys("up", "down", "j", "k"),
			key.WithHelp("up/down", "move"),
		),
		focus: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "focus"),
		),
		open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open/select"),
		),
		search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back/clear"),
		),
		help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		detailPane: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("left/right", "season/episode"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.move, k.focus, k.open, k.search, k.back, k.quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.move, k.focus, k.open},
		{k.search, k.back, k.detailPane},
		{k.help, k.quit},
	}
}
