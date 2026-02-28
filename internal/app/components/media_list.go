package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tuiflix/internal/api"
)

type mediaListItem struct {
	item api.MediaItem
}

func (i mediaListItem) Title() string {
	return i.item.Name
}

func (i mediaListItem) Description() string {
	kind := "Movie"
	if i.item.Type == "series" {
		kind = "Series"
	}
	if i.item.Year > 0 {
		return fmt.Sprintf("%s | %d", kind, i.item.Year)
	}
	return kind
}

func (i mediaListItem) FilterValue() string {
	return strings.TrimSpace(i.item.Name)
}

type MediaList struct {
	list list.Model
}

func NewMediaList(title string) MediaList {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)

	styles := list.NewDefaultItemStyles()
	styles.NormalTitle = styles.NormalTitle.Foreground(lipgloss.Color("252"))
	styles.NormalDesc = styles.NormalDesc.Foreground(mutedColor)
	styles.SelectedTitle = styles.SelectedTitle.Foreground(accentColor).Bold(true)
	styles.SelectedDesc = styles.SelectedDesc.Foreground(accentColor)
	delegate.Styles = styles

	return MediaList{list: newBaseList(title, delegate)}
}

func (m *MediaList) SetTitle(title string) {
	m.list.Title = title
}

func (m *MediaList) SetItems(items []api.MediaItem) {
	current := clamp(m.list.Index(), len(items))

	mapped := make([]list.Item, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, mediaListItem{item: item})
	}

	m.list.SetItems(mapped)
	if len(mapped) > 0 {
		m.list.Select(current)
	} else {
		m.list.ResetSelected()
	}
}

func (m *MediaList) SetCursor(index int) {
	if len(m.list.Items()) == 0 {
		m.list.ResetSelected()
		return
	}
	m.list.Select(clamp(index, len(m.list.Items())))
}

func (m MediaList) Cursor() int {
	return m.list.Index()
}

func (m MediaList) Selected() (api.MediaItem, bool) {
	selected, ok := m.list.SelectedItem().(mediaListItem)
	if !ok {
		return api.MediaItem{}, false
	}
	return selected.item, true
}

func (m *MediaList) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return cmd
}

func (m *MediaList) View(width int, height int, focused bool) string {
	m.list.SetSize(width-2, height-2)
	return renderPane(m.list.View(), width, height, focused)
}
