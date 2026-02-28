package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	footerBorder = lipgloss.Color("240")
	statusColor  = lipgloss.Color("252")
	mutedText    = lipgloss.Color("245")
	accentText   = lipgloss.Color("39")
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.height < 12 || m.width < 72 {
		return "Terminal too small for tuiflix"
	}

	topHeight := m.height - 4
	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth

	top := m.renderBrowseTop(topHeight, leftWidth, rightWidth)
	if m.mode == modeDetail {
		top = m.renderPopupOverlay(top, m.width, topHeight)
	}

	return lipgloss.JoinVertical(lipgloss.Left, top, m.renderFooter())
}

func (m *Model) renderBrowseTop(height int, leftWidth int, rightWidth int) string {
	left := m.movies.View(leftWidth, height, m.focus == focusMovies)
	right := m.right.View(rightWidth, height, m.focus == focusRight)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m *Model) renderPopupOverlay(base string, width int, height int) string {
	overlay := lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		m.renderPopup(width, height),
	)
	return mergeOverlay(base, overlay)
}

func (m *Model) renderPopup(width int, height int) string {
	if m.popup == popupSeasonEpisode {
		return m.renderSeasonEpisodePopup(width, height)
	}
	return m.renderStreamsPopup(width, height)
}

func (m *Model) renderSeasonEpisodePopup(width int, height int) string {
	popupW := min(width-6, 88)
	if popupW < 52 {
		popupW = width - 2
	}
	popupH := min(height-4, 22)
	if popupH < 14 {
		popupH = 14
	}

	listsHeight := popupH - 6
	leftW := (popupW - 5) / 2
	rightW := popupW - 5 - leftW

	pickers := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.seasons.View(leftW, listsHeight, m.focus == focusSeason),
		m.episodes.View(rightW, listsHeight, m.focus == focusEpisode),
	)

	content := []string{
		lipgloss.NewStyle().Foreground(accentText).Bold(true).Render("Choose Season & Episode"),
		lipgloss.NewStyle().Foreground(mutedText).Render(compactText(m.selected.Name, popupW-4)),
		lipgloss.NewStyle().Foreground(mutedText).Render("Tab/Left/Right to switch, arrows to move, Enter to continue"),
		pickers,
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentText).
		Padding(0, 1).
		Width(popupW).
		Height(popupH)

	return style.Render(strings.Join(content, "\n"))
}

func (m *Model) renderStreamsPopup(width int, height int) string {
	popupW := min(width-6, 104)
	if popupW < 56 {
		popupW = width - 2
	}
	popupH := min(height-4, 24)
	if popupH < 14 {
		popupH = 14
	}

	listHeight := popupH - 6
	contextLine := ""
	if m.selected.Type == "series" {
		contextLine = fmt.Sprintf("S%02dE%02d", m.currentSeason(), m.currentEpisode())
	}

	instructions := "Enter to open, Esc to close"
	if m.selected.Type == "series" {
		instructions = "Enter to open, Esc to go back"
	}

	content := []string{
		lipgloss.NewStyle().Foreground(accentText).Bold(true).Render("Choose Stream"),
		lipgloss.NewStyle().Foreground(mutedText).Render(compactText(m.selected.Name+" "+contextLine, popupW-4)),
		lipgloss.NewStyle().Foreground(mutedText).Render(instructions),
		m.streams.View(popupW-4, listHeight, m.focus == focusStreams),
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentText).
		Padding(0, 1).
		Width(popupW).
		Height(popupH)

	return style.Render(strings.Join(content, "\n"))
}

func mergeOverlay(base string, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	lineCount := max(len(baseLines), len(overlayLines))
	out := make([]string, lineCount)

	for i := 0; i < lineCount; i++ {
		var baseRunes []rune
		var overlayRunes []rune

		if i < len(baseLines) {
			baseRunes = []rune(baseLines[i])
		}
		if i < len(overlayLines) {
			overlayRunes = []rune(overlayLines[i])
		}

		width := max(len(baseRunes), len(overlayRunes))
		if width == 0 {
			out[i] = ""
			continue
		}

		merged := make([]rune, width)
		for j := 0; j < width; j++ {
			ch := ' '
			if j < len(baseRunes) {
				ch = baseRunes[j]
			}
			if j < len(overlayRunes) && overlayRunes[j] != ' ' {
				ch = overlayRunes[j]
			}
			merged[j] = ch
		}

		out[i] = string(merged)
	}

	return strings.Join(out, "\n")
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m *Model) renderFooter() string {
	if m.input.Width > m.width-16 {
		m.input.Width = max(10, m.width-16)
	}

	searchLabel := lipgloss.NewStyle().Foreground(mutedText).Render("Search")
	if m.focus == focusSearch {
		searchLabel = lipgloss.NewStyle().Foreground(accentText).Bold(true).Render("Search")
	}

	helpModel := m.help
	helpModel.Width = m.width - 4

	lines := []string{
		searchLabel + "  " + m.input.View(),
		lipgloss.NewStyle().Foreground(statusColor).Render(m.status),
		lipgloss.NewStyle().Foreground(mutedText).Render(helpModel.View(m.keys)),
	}

	box := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(footerBorder).
		Width(m.width)

	return box.Render(strings.Join(lines, "\n"))
}
