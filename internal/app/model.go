package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"tuiflix/internal/api"
	"tuiflix/internal/app/components"
)

type viewMode int
type popupMode int
type focusArea int

const (
	modeBrowse viewMode = iota
	modeDetail
)

const (
	popupNone popupMode = iota
	popupSeasonEpisode
	popupStreams
)

const (
	focusMovies focusArea = iota
	focusRight
	focusSearch
	focusStreams
	focusSeason
	focusEpisode
)

type Model struct {
	client *api.Client

	width  int
	height int

	mode  viewMode
	popup popupMode
	focus focusArea

	input textinput.Model
	help  help.Model
	keys  keyMap

	movies   components.MediaList
	right    components.MediaList
	streams  components.StreamList
	seasons  components.NumberList
	episodes components.NumberList

	moviesData         []api.MediaItem
	showsData          []api.MediaItem
	searchMovieResults []api.MediaItem
	searchShowResults  []api.MediaItem
	showSearch         bool

	selected api.MediaItem

	episodesBySeason map[int][]int
	streamsReqKey    string

	status string
}

func NewModel(client *api.Client) Model {
	input := textinput.New()
	input.Placeholder = "Search movies and TV"
	input.CharLimit = 140
	input.Width = 48
	input.Prompt = ""
	input.Focus()

	h := help.New()
	h.ShowAll = false

	status := "Loading popular titles..."
	if !client.RealDebridEnabled() {
		status = "REALDEBRID not found: magnet links will open directly in IINA"
	}

	movies := components.NewMediaList("Popular Movies")
	right := components.NewMediaList("Popular TV Shows")
	streams := components.NewStreamList("Streams")
	seasons := components.NewNumberList("Seasons")
	episodes := components.NewNumberList("Episodes")
	seasons.SetItems([]int{1})
	episodes.SetItems([]int{1})

	return Model{
		client:             client,
		mode:               modeBrowse,
		popup:              popupNone,
		focus:              focusSearch,
		input:              input,
		help:               h,
		keys:               newKeyMap(),
		movies:             movies,
		right:              right,
		streams:            streams,
		seasons:            seasons,
		episodes:           episodes,
		moviesData:         []api.MediaItem{},
		showsData:          []api.MediaItem{},
		episodesBySeason:   map[int][]int{1: []int{1}},
		searchMovieResults: []api.MediaItem{},
		searchShowResults:  []api.MediaItem{},
		status:             status,
	}
}

func (m Model) Init() tea.Cmd {
	return loadPopularCmd(m.client)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(10, m.width-16)
		return m, nil

	case popularLoadedMsg:
		if msg.err != nil {
			m.status = "Failed to load popular titles: " + msg.err.Error()
			return m, nil
		}
		m.moviesData = msg.movies
		m.showsData = msg.shows
		m.syncBrowsePanes()
		if m.status == "Loading popular titles..." {
			m.status = "Browse with arrows/tab, enter opens details"
		}
		return m, nil

	case searchLoadedMsg:
		if msg.query != strings.TrimSpace(m.input.Value()) {
			return m, nil
		}
		if msg.err != nil {
			m.status = "Search failed: " + msg.err.Error()
			return m, nil
		}

		movieResults, showResults := splitSearchResults(msg.results)
		m.searchMovieResults = movieResults
		m.searchShowResults = showResults
		m.showSearch = true
		m.syncBrowsePanes()

		if len(movieResults) > 0 {
			m.setFocus(focusMovies)
		} else if len(showResults) > 0 {
			m.setFocus(focusRight)
		} else {
			m.setFocus(focusMovies)
		}

		m.status = fmt.Sprintf("Found %d movie(s), %d series", len(movieResults), len(showResults))
		return m, nil

	case episodesLoadedMsg:
		if m.mode != modeDetail || msg.itemID != m.selected.ID {
			return m, nil
		}
		if msg.err != nil {
			m.status = "Failed to load season/episode metadata: " + msg.err.Error()
			return m, nil
		}

		m.episodesBySeason = msg.bySeason
		seasonOptions := sortedMapKeys(msg.bySeason)
		if len(seasonOptions) == 0 {
			seasonOptions = []int{1}
		}

		prevSeason := m.currentSeason()
		m.seasons.SetItems(seasonOptions)
		m.seasons.SetCursor(indexOfInt(seasonOptions, prevSeason))
		m.syncEpisodeOptions(true)
		m.status = "Pick season/episode, then press Enter"
		return m, nil

	case streamsLoadedMsg:
		if m.mode != modeDetail || msg.key != m.streamsReqKey {
			return m, nil
		}
		if msg.err != nil {
			m.streams.SetItems(nil)
			m.status = "Failed to load streams: " + msg.err.Error()
			return m, nil
		}
		m.streams.SetItems(msg.streams)
		if len(msg.streams) == 0 {
			m.status = "No streams found for this selection"
		} else {
			m.status = fmt.Sprintf("Loaded %d stream(s). Enter opens in IINA", len(msg.streams))
		}
		return m, nil

	case streamOpenedMsg:
		if msg.err != nil {
			m.status = "Unable to open stream: " + msg.err.Error()
			return m, nil
		}
		m.status = "Opening stream in IINA"
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case "/":
			if m.mode == modeBrowse {
				m.setFocus(focusSearch)
			}
			return m, nil
		}

		if m.mode == modeBrowse {
			return m.updateBrowseKey(msg)
		}
		return m.updateDetailKey(msg)
	}

	if m.mode == modeBrowse && m.focus == focusSearch {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.cycleBrowseFocus(false)
		return m, nil
	case "shift+tab":
		m.cycleBrowseFocus(true)
		return m, nil
	case "left":
		m.setFocus(focusMovies)
		return m, nil
	case "right":
		m.setFocus(focusRight)
		return m, nil
	case "esc":
		if m.showSearch {
			m.showSearch = false
			m.searchMovieResults = nil
			m.searchShowResults = nil
			m.syncBrowsePanes()
			m.status = "Search cleared"
		}
		return m, nil
	case "enter":
		if m.focus == focusSearch {
			query := strings.TrimSpace(m.input.Value())
			if query == "" {
				m.showSearch = false
				m.searchMovieResults = nil
				m.searchShowResults = nil
				m.syncBrowsePanes()
				m.status = "Search cleared"
				return m, nil
			}
			m.status = "Searching..."
			return m, loadSearchCmd(m.client, query)
		}

		item, ok := m.currentBrowseSelection()
		if !ok {
			return m, nil
		}
		return m.openDetail(item)
	}

	if m.focus == focusSearch {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, m.updateBrowseList(msg)
}

func (m Model) updateDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.popup == popupSeasonEpisode {
		return m.updateSeasonEpisodePopupKey(msg)
	}
	return m.updateStreamsPopupKey(msg)
}

func (m Model) updateSeasonEpisodePopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "right":
		m.setFocus(focusEpisode)
		return m, nil
	case "shift+tab", "left":
		m.setFocus(focusSeason)
		return m, nil
	case "esc":
		return m.closeDetail()
	case "enter":
		m.popup = popupStreams
		m.streams.SetItems(nil)
		m.setFocus(focusStreams)
		return m, m.reloadStreamsCmd()
	}

	return m, m.updateDetailList(msg)
}

func (m Model) updateStreamsPopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.selected.Type == "series" {
			m.popup = popupSeasonEpisode
			m.setFocus(focusSeason)
			m.status = "Pick season/episode, then press Enter"
			return m, nil
		}
		return m.closeDetail()
	case "enter":
		stream, ok := m.streams.Selected()
		if !ok {
			return m, nil
		}
		m.status = "Resolving stream URL..."
		return m, openStreamCmd(m.client, stream)
	}

	return m, m.updateDetailList(msg)
}

func (m *Model) updateBrowseList(msg tea.Msg) tea.Cmd {
	switch m.focus {
	case focusMovies:
		return m.movies.Update(msg)
	case focusRight:
		return m.right.Update(msg)
	default:
		return nil
	}
}

func (m *Model) updateDetailList(msg tea.Msg) tea.Cmd {
	switch m.focus {
	case focusStreams:
		return m.streams.Update(msg)
	case focusSeason:
		previous := m.currentSeason()
		cmd := m.seasons.Update(msg)
		if previous != m.currentSeason() {
			m.syncEpisodeOptions(true)
		}
		return cmd
	case focusEpisode:
		return m.episodes.Update(msg)
	default:
		return nil
	}
}

func (m Model) openDetail(item api.MediaItem) (tea.Model, tea.Cmd) {
	m.mode = modeDetail
	m.selected = item
	m.streams.SetTitle("Streams: " + compactText(item.Name, 40))
	m.streams.SetItems(nil)
	m.episodesBySeason = map[int][]int{1: []int{1}}
	m.seasons.SetItems([]int{1})
	m.seasons.SetCursor(0)
	m.episodes.SetItems([]int{1})
	m.episodes.SetCursor(0)

	if item.Type == "series" {
		m.popup = popupSeasonEpisode
		m.setFocus(focusSeason)
		m.status = "Loading seasons and episodes..."
		return m, loadEpisodesCmd(m.client, item.ID)
	}

	m.popup = popupStreams
	m.setFocus(focusStreams)
	m.status = "Loading streams..."
	return m, m.reloadStreamsCmd()
}

func (m Model) closeDetail() (tea.Model, tea.Cmd) {
	m.mode = modeBrowse
	m.popup = popupNone
	m.setFocus(focusRight)
	m.status = "Back to browse"
	return m, nil
}

func (m *Model) syncBrowsePanes() {
	if m.showSearch {
		m.movies.SetTitle(fmt.Sprintf("Movie Results (%d)", len(m.searchMovieResults)))
		m.movies.SetItems(m.searchMovieResults)
		m.right.SetTitle(fmt.Sprintf("Series Results (%d)", len(m.searchShowResults)))
		m.right.SetItems(m.searchShowResults)
		return
	}

	m.movies.SetTitle("Popular Movies")
	m.movies.SetItems(m.moviesData)
	m.right.SetTitle("Popular TV Shows")
	m.right.SetItems(m.showsData)
}

func splitSearchResults(items []api.MediaItem) ([]api.MediaItem, []api.MediaItem) {
	movies := make([]api.MediaItem, 0)
	shows := make([]api.MediaItem, 0)

	for _, item := range items {
		if item.Type == "series" {
			shows = append(shows, item)
			continue
		}
		movies = append(movies, item)
	}

	return movies, shows
}

func (m *Model) syncEpisodeOptions(resetCursor bool) {
	season := m.currentSeason()
	episodes := append([]int(nil), m.episodesBySeason[season]...)
	if len(episodes) == 0 {
		episodes = []int{1}
	}

	previous := m.currentEpisode()
	m.episodes.SetItems(episodes)
	if resetCursor {
		m.episodes.SetCursor(0)
		return
	}
	m.episodes.SetCursor(indexOfInt(episodes, previous))
}

func (m *Model) reloadStreamsCmd() tea.Cmd {
	season := m.currentSeason()
	episode := m.currentEpisode()
	key := fmt.Sprintf("%s:%d:%d", m.selected.ID, season, episode)
	m.streamsReqKey = key
	m.status = fmt.Sprintf("Loading streams for S%02dE%02d...", season, episode)
	return loadStreamsCmd(m.client, m.selected, season, episode, key)
}

func (m *Model) cycleBrowseFocus(reverse bool) {
	order := []focusArea{focusMovies, focusRight, focusSearch}
	m.focus = cycleInOrder(order, m.focus, reverse)
	m.syncInputFocus()
}

func (m *Model) setFocus(area focusArea) {
	m.focus = area
	m.syncInputFocus()
}

func (m *Model) syncInputFocus() {
	if m.mode == modeBrowse && m.focus == focusSearch {
		m.input.Focus()
		return
	}
	m.input.Blur()
}

func (m Model) currentBrowseSelection() (api.MediaItem, bool) {
	if m.focus == focusMovies {
		if item, ok := m.movies.Selected(); ok {
			return item, true
		}
	}
	if m.focus == focusRight {
		if item, ok := m.right.Selected(); ok {
			return item, true
		}
	}
	if item, ok := m.movies.Selected(); ok {
		return item, true
	}
	return m.right.Selected()
}

func (m Model) currentSeason() int {
	season, ok := m.seasons.Selected()
	if !ok {
		return 1
	}
	return season
}

func (m Model) currentEpisode() int {
	episode, ok := m.episodes.Selected()
	if !ok {
		return 1
	}
	return episode
}

func cycleInOrder(order []focusArea, current focusArea, reverse bool) focusArea {
	if len(order) == 0 {
		return current
	}

	idx := 0
	for i, value := range order {
		if value == current {
			idx = i
			break
		}
	}

	if reverse {
		idx--
		if idx < 0 {
			idx = len(order) - 1
		}
		return order[idx]
	}

	idx = (idx + 1) % len(order)
	return order[idx]
}

func sortedMapKeys(values map[int][]int) []int {
	keys := make([]int, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func indexOfInt(values []int, target int) int {
	for idx, value := range values {
		if value == target {
			return idx
		}
	}
	return 0
}

func compactText(input string, width int) string {
	if width <= 0 {
		return ""
	}
	trimmed := strings.TrimSpace(input)
	if len(trimmed) <= width {
		return trimmed
	}
	if width <= 3 {
		return trimmed[:width]
	}
	return trimmed[:width-3] + "..."
}

func maybeYear(year int) string {
	if year <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d", year)
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
