package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type numberItem struct {
	value int
}

func (i numberItem) Title() string {
	return fmt.Sprintf("%d", i.value)
}

func (i numberItem) Description() string {
	return ""
}

func (i numberItem) FilterValue() string {
	return i.Title()
}

type NumberList struct {
	list list.Model
}

func NewNumberList(title string) NumberList {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)

	styles := list.NewDefaultItemStyles()
	styles.NormalTitle = styles.NormalTitle.Foreground(lipgloss.Color("252"))
	styles.SelectedTitle = styles.SelectedTitle.Foreground(accentColor).Bold(true)
	delegate.Styles = styles

	lm := newBaseList(title, delegate)
	lm.SetShowPagination(false)

	return NumberList{list: lm}
}

func (n *NumberList) SetTitle(title string) {
	n.list.Title = title
}

func (n *NumberList) SetItems(values []int) {
	current := clamp(n.list.Index(), len(values))

	mapped := make([]list.Item, 0, len(values))
	for _, value := range values {
		mapped = append(mapped, numberItem{value: value})
	}

	n.list.SetItems(mapped)
	if len(mapped) > 0 {
		n.list.Select(current)
	} else {
		n.list.ResetSelected()
	}
}

func (n *NumberList) SetCursor(index int) {
	if len(n.list.Items()) == 0 {
		n.list.ResetSelected()
		return
	}
	n.list.Select(clamp(index, len(n.list.Items())))
}

func (n NumberList) Cursor() int {
	return n.list.Index()
}

func (n NumberList) Selected() (int, bool) {
	selected, ok := n.list.SelectedItem().(numberItem)
	if !ok {
		return 0, false
	}
	return selected.value, true
}

func (n *NumberList) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	n.list, cmd = n.list.Update(msg)
	return cmd
}

func (n *NumberList) View(width int, height int, focused bool) string {
	n.list.SetSize(width-2, height-2)
	return renderPane(n.list.View(), width, height, focused)
}
