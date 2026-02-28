package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tuiflix/internal/api"
)

type streamListItem struct {
	stream api.Stream
}

func (i streamListItem) Title() string {
	base := strings.TrimSpace(i.stream.Title)
	if base == "" {
		base = strings.TrimSpace(i.stream.Name)
	}
	if base == "" {
		base = "Torrent stream"
	}
	base = strings.ReplaceAll(base, "\n", " | ")
	return base
}

func (i streamListItem) Description() string {
	provider := strings.TrimSpace(i.stream.Name)
	if provider == "" {
		provider = "unknown"
	}
	kind := "Magnet"
	if strings.HasPrefix(strings.ToLower(i.stream.URL), "http") {
		kind = "HTTP"
	}
	return provider + " | " + kind
}

func (i streamListItem) FilterValue() string {
	return i.Title()
}

type StreamList struct {
	list list.Model
}

func NewStreamList(title string) StreamList {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)

	styles := list.NewDefaultItemStyles()
	styles.NormalTitle = styles.NormalTitle.Foreground(lipgloss.Color("252"))
	styles.NormalDesc = styles.NormalDesc.Foreground(mutedColor)
	styles.SelectedTitle = styles.SelectedTitle.Foreground(accentColor).Bold(true)
	styles.SelectedDesc = styles.SelectedDesc.Foreground(accentColor)
	delegate.Styles = styles

	return StreamList{list: newBaseList(title, delegate)}
}

func (s *StreamList) SetTitle(title string) {
	s.list.Title = title
}

func (s *StreamList) SetItems(items []api.Stream) {
	current := clamp(s.list.Index(), len(items))

	mapped := make([]list.Item, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, streamListItem{stream: item})
	}

	s.list.SetItems(mapped)
	if len(mapped) > 0 {
		s.list.Select(current)
	} else {
		s.list.ResetSelected()
	}
}

func (s *StreamList) SetCursor(index int) {
	if len(s.list.Items()) == 0 {
		s.list.ResetSelected()
		return
	}
	s.list.Select(clamp(index, len(s.list.Items())))
}

func (s StreamList) Cursor() int {
	return s.list.Index()
}

func (s StreamList) Selected() (api.Stream, bool) {
	selected, ok := s.list.SelectedItem().(streamListItem)
	if !ok {
		return api.Stream{}, false
	}
	return selected.stream, true
}

func (s *StreamList) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

func (s *StreamList) View(width int, height int, focused bool) string {
	s.list.SetSize(width-2, height-2)
	return renderPane(s.list.View(), width, height, focused)
}
