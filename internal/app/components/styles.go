package components

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

var (
	accentColor = lipgloss.Color("39")
	mutedColor  = lipgloss.Color("245")
	focusBorder = lipgloss.Color("39")
	blurBorder  = lipgloss.Color("240")
)

func newBaseList(title string, delegate list.ItemDelegate) list.Model {
	lm := list.New([]list.Item{}, delegate, 0, 0)
	lm.Title = title
	lm.SetShowFilter(false)
	lm.SetFilteringEnabled(false)
	lm.SetShowHelp(false)
	lm.SetShowStatusBar(false)
	lm.SetShowPagination(true)
	lm.DisableQuitKeybindings()

	styles := list.DefaultStyles()
	styles.Title = styles.Title.Foreground(accentColor).Bold(true)
	styles.TitleBar = styles.TitleBar.Padding(0, 1)
	styles.PaginationStyle = styles.PaginationStyle.Foreground(mutedColor)
	styles.NoItems = styles.NoItems.Foreground(mutedColor).PaddingLeft(1)
	lm.Styles = styles

	return lm
}

func renderPane(content string, width int, height int, focused bool) string {
	if width < 3 {
		width = 3
	}
	if height < 3 {
		height = 3
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(blurBorder).
		Width(width).
		Height(height)

	if focused {
		style = style.BorderForeground(focusBorder)
	}

	return style.Render(content)
}

func clamp(index int, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}
