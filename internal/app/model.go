package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"tuiflix/internal/api"
	"tuiflix/internal/player"
)

type viewMode int
type focusArea int

const (
	modeBrowse viewMode = iota
	modeDetail
)

const (
	focusMovies focusArea = iota
	focusRight
	focusSearch
	focusStreams
	focusSeason
	focusEpisode
)

type popularLoadedMsg struct {
	movies []api.MediaItem
	shows  []api.MediaItem
	err    error
}

type searchLoadedMsg struct {
	query   string
	results []api.MediaItem
	err     error
}

type episodesLoadedMsg struct {
	itemID   string
	bySeason map[int][]int
	err      error
}

type streamsLoadedMsg struct {
	key     string
	streams []api.Stream
	err     error
}

type streamOpenedMsg struct {
	err error
}

type Model struct {
	client *api.Client

	width  int
	height int

	mode  viewMode
	focus focusArea

	input textinput.Model

	movies        []api.MediaItem
	shows         []api.MediaItem
	searchResults []api.MediaItem
	showSearch    bool

	movieCursor int
	rightCursor int

	selected      api.MediaItem
	streams       []api.Stream
	streamCursor  int
	streamsReqKey string

	episodesBySeason map[int][]int
	seasonOptions    []int
	episodeOptions   []int
	seasonCursor     int
	episodeCursor    int

	status string
}

func NewModel(client *api.Client) Model {
	input := textinput.New()
	input.Placeholder = "Search movies and TV..."
	input.CharLimit = 140
	input.Width = 48
	input.Prompt = ""
	input.Focus()

	status := "Loading popular titles..."
	if !client.RealDebridEnabled() {
		status = "REALDEBRID not found: magnet links will open directly in IINA"
	}

	return Model{
		client:     client,
		mode:       modeBrowse,
		focus:      focusSearch,
		input:      input,
		status:     status,
		movies:     []api.MediaItem{},
		shows:      []api.MediaItem{},
		streams:    []api.Stream{},
		showSearch: false,
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
		if m.input.Width > m.width-16 {
			m.input.Width = max(10, m.width-16)
		}
		return m, nil

	case popularLoadedMsg:
		if msg.err != nil {
			m.status = "Failed to load popular titles: " + msg.err.Error()
			return m, nil
		}
		m.movies = msg.movies
		m.shows = msg.shows
		if m.status == "Loading popular titles..." {
			m.status = "Browse with arrows/tab, press enter to open, esc to go back"
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
		m.searchResults = msg.results
		m.showSearch = true
		m.rightCursor = 0
		m.focus = focusRight
		m.input.Blur()
		m.status = fmt.Sprintf("Found %d result(s)", len(msg.results))
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
		m.seasonOptions = sortedMapKeys(msg.bySeason)
		if len(m.seasonOptions) == 0 {
			m.seasonOptions = []int{1}
		}
		if m.seasonCursor >= len(m.seasonOptions) {
			m.seasonCursor = 0
		}
		m.syncEpisodeOptions()
		return m, m.reloadStreamsCmd()

	case streamsLoadedMsg:
		if m.mode != modeDetail || msg.key != m.streamsReqKey {
			return m, nil
		}
		if msg.err != nil {
			m.streams = nil
			m.streamCursor = 0
			m.status = "Failed to load streams: " + msg.err.Error()
			return m, nil
		}
		m.streams = msg.streams
		if m.streamCursor >= len(m.streams) {
			m.streamCursor = max(0, len(m.streams)-1)
		}
		if len(m.streams) == 0 {
			m.status = "No streams found for this selection"
		} else {
			m.status = fmt.Sprintf("Loaded %d stream(s). Enter opens in IINA", len(m.streams))
		}
		return m, nil

	case streamOpenedMsg:
		if msg.err != nil {
			m.status = "Unable to open stream: " + msg.err.Error()
			return m, nil
		}
		m.status = "Opening stream in IINA"
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if key := msg.String(); key == "ctrl+c" || key == "q" {
			return m, tea.Quit
		}

		if key := msg.String(); key == "/" {
			m.setFocus(focusSearch)
			return m, nil
		}

		if m.mode == modeBrowse {
			return m.updateBrowseKey(msg)
		}
		return m.updateDetailKey(msg)
	}

	if m.focus == focusSearch {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.height < 8 || m.width < 60 {
		return "Terminal too small for tuiflix"
	}

	searchHeight := 3
	topHeight := m.height - searchHeight
	leftWidth := (m.width - 1) / 2
	rightWidth := m.width - leftWidth - 1

	var topLines []string
	if m.mode == modeBrowse {
		topLines = m.renderBrowseTop(topHeight, leftWidth, rightWidth)
	} else {
		topLines = m.renderDetailTop(topHeight, leftWidth, rightWidth)
	}

	bottom := m.renderBottom(searchHeight)
	return strings.Join(append(topLines, bottom...), "\n")
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
	case "up":
		m.moveBrowseCursor(-1)
		return m, nil
	case "down":
		m.moveBrowseCursor(1)
		return m, nil
	case "esc":
		if m.showSearch {
			m.showSearch = false
			m.searchResults = nil
			m.rightCursor = 0
			m.status = "Search cleared"
		}
		return m, nil
	case "enter":
		if m.focus == focusSearch {
			query := strings.TrimSpace(m.input.Value())
			if query == "" {
				m.showSearch = false
				m.searchResults = nil
				m.rightCursor = 0
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

	return m, nil
}

func (m Model) updateDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.cycleDetailFocus(false)
		return m, nil
	case "shift+tab":
		m.cycleDetailFocus(true)
		return m, nil
	case "left":
		if m.selected.Type == "series" {
			if m.focus == focusSeason || m.focus == focusEpisode {
				m.setFocus(focusStreams)
			}
		}
		return m, nil
	case "right":
		if m.selected.Type == "series" && m.focus == focusStreams {
			m.setFocus(focusSeason)
		}
		return m, nil
	case "esc":
		m.mode = modeBrowse
		m.setFocus(focusRight)
		m.status = "Back to browse"
		return m, nil
	case "up":
		return m.detailMove(-1)
	case "down":
		return m.detailMove(1)
	case "enter":
		if m.focus == focusStreams {
			if len(m.streams) == 0 || m.streamCursor >= len(m.streams) {
				return m, nil
			}
			m.status = "Resolving stream URL..."
			return m, openStreamCmd(m.client, m.streams[m.streamCursor])
		}
		if m.focus == focusSearch {
			query := strings.TrimSpace(m.input.Value())
			if query == "" {
				return m, nil
			}
			m.mode = modeBrowse
			m.setFocus(focusSearch)
			m.status = "Searching..."
			return m, loadSearchCmd(m.client, query)
		}
	}

	if m.focus == focusSearch {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) detailMove(delta int) (tea.Model, tea.Cmd) {
	if m.focus == focusStreams {
		m.streamCursor = clampCursor(m.streamCursor+delta, len(m.streams))
		return m, nil
	}

	if m.selected.Type != "series" {
		return m, nil
	}

	if m.focus == focusSeason {
		prevSeason := m.currentSeason()
		m.seasonCursor = clampCursor(m.seasonCursor+delta, len(m.seasonOptions))
		if prevSeason != m.currentSeason() {
			m.episodeCursor = 0
			m.syncEpisodeOptions()
			return m, m.reloadStreamsCmd()
		}
		return m, nil
	}

	if m.focus == focusEpisode {
		prevEpisode := m.currentEpisode()
		m.episodeCursor = clampCursor(m.episodeCursor+delta, len(m.episodeOptions))
		if prevEpisode != m.currentEpisode() {
			return m, m.reloadStreamsCmd()
		}
	}

	return m, nil
}

func (m Model) openDetail(item api.MediaItem) (tea.Model, tea.Cmd) {
	m.mode = modeDetail
	m.selected = item
	m.streams = nil
	m.streamCursor = 0
	m.episodesBySeason = map[int][]int{1: []int{1}}
	m.seasonOptions = []int{1}
	m.episodeOptions = []int{1}
	m.seasonCursor = 0
	m.episodeCursor = 0
	m.setFocus(focusStreams)
	m.status = "Loading streams..."

	cmds := []tea.Cmd{m.reloadStreamsCmd()}
	if item.Type == "series" {
		cmds = append(cmds, loadEpisodesCmd(m.client, item.ID))
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) syncEpisodeOptions() {
	season := m.currentSeason()
	episodes := append([]int(nil), m.episodesBySeason[season]...)
	if len(episodes) == 0 {
		episodes = []int{1}
	}
	m.episodeOptions = episodes
	m.episodeCursor = clampCursor(m.episodeCursor, len(m.episodeOptions))
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

func (m *Model) cycleDetailFocus(reverse bool) {
	order := []focusArea{focusStreams, focusSearch}
	if m.selected.Type == "series" {
		order = []focusArea{focusStreams, focusSeason, focusEpisode, focusSearch}
	}
	m.focus = cycleInOrder(order, m.focus, reverse)
	m.syncInputFocus()
}

func (m *Model) setFocus(area focusArea) {
	m.focus = area
	m.syncInputFocus()
}

func (m *Model) syncInputFocus() {
	if m.focus == focusSearch {
		m.input.Focus()
		return
	}
	m.input.Blur()
}

func (m *Model) moveBrowseCursor(delta int) {
	if m.focus == focusMovies {
		m.movieCursor = clampCursor(m.movieCursor+delta, len(m.movies))
		return
	}
	if m.focus == focusRight {
		m.rightCursor = clampCursor(m.rightCursor+delta, len(m.currentRightItems()))
	}
}

func (m Model) currentBrowseSelection() (api.MediaItem, bool) {
	if m.focus == focusMovies {
		if m.movieCursor >= 0 && m.movieCursor < len(m.movies) {
			return m.movies[m.movieCursor], true
		}
		return api.MediaItem{}, false
	}

	items := m.currentRightItems()
	if m.rightCursor >= 0 && m.rightCursor < len(items) {
		return items[m.rightCursor], true
	}

	return api.MediaItem{}, false
}

func (m Model) currentRightItems() []api.MediaItem {
	if m.showSearch {
		return m.searchResults
	}
	return m.shows
}

func (m Model) currentSeason() int {
	if len(m.seasonOptions) == 0 {
		return 1
	}
	return m.seasonOptions[clampCursor(m.seasonCursor, len(m.seasonOptions))]
}

func (m Model) currentEpisode() int {
	if len(m.episodeOptions) == 0 {
		return 1
	}
	return m.episodeOptions[clampCursor(m.episodeCursor, len(m.episodeOptions))]
}

func (m Model) renderBrowseTop(h int, leftW int, rightW int) []string {
	left := m.renderMediaPane("Popular Movies", m.movies, m.movieCursor, h, leftW, m.focus == focusMovies)
	rightTitle := "Popular TV Shows"
	if m.showSearch {
		rightTitle = fmt.Sprintf("Search Results (%d)", len(m.searchResults))
	}
	right := m.renderMediaPane(rightTitle, m.currentRightItems(), m.rightCursor, h, rightW, m.focus == focusRight)

	lines := make([]string, 0, h)
	for i := 0; i < h; i++ {
		lines = append(lines, padRight(left[i], leftW)+"|"+padRight(right[i], rightW))
	}

	return lines
}

func (m Model) renderDetailTop(h int, leftW int, rightW int) []string {
	streamsTitle := "Streams: " + compactText(m.selected.Name, leftW-10)
	left := m.renderStreamPane(streamsTitle, m.streams, m.streamCursor, h, leftW, m.focus == focusStreams)

	var right []string
	if m.selected.Type == "series" {
		right = m.renderSeasonEpisodePane(h, rightW)
	} else {
		right = m.renderInfoPane(h, rightW)
	}

	lines := make([]string, 0, h)
	for i := 0; i < h; i++ {
		lines = append(lines, padRight(left[i], leftW)+"|"+padRight(right[i], rightW))
	}

	return lines
}

func (m Model) renderMediaPane(title string, items []api.MediaItem, cursor int, h int, w int, focused bool) []string {
	lines := make([]string, h)
	head := title
	if focused {
		head = "[x] " + head
	} else {
		head = "[ ] " + head
	}
	lines[0] = compactText(head, w)
	lines[1] = strings.Repeat("-", max(1, w))

	rows := h - 2
	start := scrollStart(len(items), cursor, rows)
	for row := 0; row < rows; row++ {
		idx := start + row
		lineAt := row + 2
		if idx >= len(items) {
			if row == 0 && len(items) == 0 {
				lines[lineAt] = "(empty)"
			}
			continue
		}

		prefix := "  "
		if idx == cursor {
			if focused {
				prefix = "> "
			} else {
				prefix = "* "
			}
		}

		label := prefix + itemLabel(items[idx])
		lines[lineAt] = compactText(label, w)
	}

	return lines
}

func (m Model) renderStreamPane(title string, streams []api.Stream, cursor int, h int, w int, focused bool) []string {
	lines := make([]string, h)
	head := "[ ] " + title
	if focused {
		head = "[x] " + title
	}
	lines[0] = compactText(head, w)
	lines[1] = strings.Repeat("-", max(1, w))

	rows := h - 2
	start := scrollStart(len(streams), cursor, rows)
	for row := 0; row < rows; row++ {
		idx := start + row
		lineAt := row + 2
		if idx >= len(streams) {
			if row == 0 && len(streams) == 0 {
				lines[lineAt] = "(no streams)"
			}
			continue
		}

		prefix := "  "
		if idx == cursor {
			if focused {
				prefix = "> "
			} else {
				prefix = "* "
			}
		}

		label := streamLabel(streams[idx])
		lines[lineAt] = compactText(prefix+label, w)
	}

	return lines
}

func (m Model) renderSeasonEpisodePane(h int, w int) []string {
	seasonHeight := h / 2
	episodeHeight := h - seasonHeight

	season := renderIntList("Seasons", m.seasonOptions, m.seasonCursor, seasonHeight, w, m.focus == focusSeason)
	episode := renderIntList("Episodes", m.episodeOptions, m.episodeCursor, episodeHeight, w, m.focus == focusEpisode)

	return append(season, episode...)
}

func (m Model) renderInfoPane(h int, w int) []string {
	lines := make([]string, h)
	lines[0] = "Details"
	lines[1] = strings.Repeat("-", max(1, w))

	meta := []string{
		"Type: " + m.selected.Type,
		"Year: " + maybeYear(m.selected.Year),
		"",
		"Enter on a stream",
		"to open in IINA",
	}

	for i := 0; i < len(meta) && i+2 < len(lines); i++ {
		lines[i+2] = compactText(meta[i], w)
	}

	return lines
}

func (m Model) renderBottom(h int) []string {
	lines := make([]string, h)
	lines[0] = strings.Repeat("=", max(1, m.width))

	prefix := "Search"
	if m.focus == focusSearch {
		prefix = "Search*"
	}
	inputText := strings.TrimSpace(m.input.Value())
	if inputText == "" {
		inputText = "<" + m.input.Placeholder + ">"
	}
	if m.focus == focusSearch {
		inputText += "_"
	}
	lines[1] = compactText(prefix+": "+inputText, m.width)
	lines[2] = compactText(m.status, m.width)

	return lines
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if m.width == 0 || m.height == 0 {
		return m, nil
	}

	searchStart := m.height - 3
	if msg.Y >= searchStart {
		m.setFocus(focusSearch)
		return m, nil
	}

	if m.mode == modeBrowse {
		return m.handleBrowseMouse(msg)
	}

	return m.handleDetailMouse(msg)
}

func (m Model) handleBrowseMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	topHeight := m.height - 3
	leftWidth := (m.width - 1) / 2

	if msg.X < leftWidth {
		if msg.Y < 2 {
			m.setFocus(focusMovies)
			return m, nil
		}
		rows := topHeight - 2
		start := scrollStart(len(m.movies), m.movieCursor, rows)
		idx := start + (msg.Y - 2)
		if idx >= 0 && idx < len(m.movies) {
			m.movieCursor = idx
			m.setFocus(focusMovies)
			return m.openDetail(m.movies[idx])
		}
		return m, nil
	}

	if msg.X > leftWidth {
		items := m.currentRightItems()
		if msg.Y < 2 {
			m.setFocus(focusRight)
			return m, nil
		}
		rows := topHeight - 2
		start := scrollStart(len(items), m.rightCursor, rows)
		idx := start + (msg.Y - 2)
		if idx >= 0 && idx < len(items) {
			m.rightCursor = idx
			m.setFocus(focusRight)
			return m.openDetail(items[idx])
		}
	}

	return m, nil
}

func (m Model) handleDetailMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	topHeight := m.height - 3
	leftWidth := (m.width - 1) / 2

	if msg.X < leftWidth {
		if msg.Y < 2 {
			m.setFocus(focusStreams)
			return m, nil
		}
		rows := topHeight - 2
		start := scrollStart(len(m.streams), m.streamCursor, rows)
		idx := start + (msg.Y - 2)
		if idx >= 0 && idx < len(m.streams) {
			m.streamCursor = idx
			m.setFocus(focusStreams)
			return m, openStreamCmd(m.client, m.streams[idx])
		}
		return m, nil
	}

	if m.selected.Type != "series" {
		return m, nil
	}

	rightY := msg.Y
	seasonHeight := topHeight / 2
	if rightY < seasonHeight {
		if rightY < 2 {
			m.setFocus(focusSeason)
			return m, nil
		}
		rows := seasonHeight - 2
		start := scrollStart(len(m.seasonOptions), m.seasonCursor, rows)
		idx := start + (rightY - 2)
		if idx >= 0 && idx < len(m.seasonOptions) {
			if idx != m.seasonCursor {
				m.seasonCursor = idx
				m.episodeCursor = 0
				m.syncEpisodeOptions()
				m.setFocus(focusSeason)
				return m, m.reloadStreamsCmd()
			}
			m.setFocus(focusSeason)
		}
		return m, nil
	}

	episodeY := rightY - seasonHeight
	if episodeY < 2 {
		m.setFocus(focusEpisode)
		return m, nil
	}
	rows := (topHeight - seasonHeight) - 2
	start := scrollStart(len(m.episodeOptions), m.episodeCursor, rows)
	idx := start + (episodeY - 2)
	if idx >= 0 && idx < len(m.episodeOptions) {
		if idx != m.episodeCursor {
			m.episodeCursor = idx
			m.setFocus(focusEpisode)
			return m, m.reloadStreamsCmd()
		}
		m.setFocus(focusEpisode)
	}

	return m, nil
}

func loadPopularCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		movies, shows, err := client.FetchPopular(ctx)
		return popularLoadedMsg{movies: movies, shows: shows, err: err}
	}
}

func loadSearchCmd(client *api.Client, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		results, err := client.Search(ctx, query)
		return searchLoadedMsg{query: query, results: results, err: err}
	}
}

func loadEpisodesCmd(client *api.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		bySeason, err := client.FetchSeriesEpisodes(ctx, id)
		return episodesLoadedMsg{itemID: id, bySeason: bySeason, err: err}
	}
}

func loadStreamsCmd(client *api.Client, item api.MediaItem, season int, episode int, key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		streams, err := client.FetchStreams(ctx, item, season, episode)
		return streamsLoadedMsg{key: key, streams: streams, err: err}
	}
}

func openStreamCmd(client *api.Client, stream api.Stream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		playableURL, err := client.ResolvePlayableURL(ctx, stream)
		if err != nil {
			return streamOpenedMsg{err: err}
		}

		if err := player.OpenIINA(playableURL); err != nil {
			return streamOpenedMsg{err: err}
		}

		return streamOpenedMsg{}
	}
}

func renderIntList(title string, values []int, cursor int, h int, w int, focused bool) []string {
	if h < 2 {
		h = 2
	}

	lines := make([]string, h)
	head := "[ ] " + title
	if focused {
		head = "[x] " + title
	}
	lines[0] = compactText(head, w)
	lines[1] = strings.Repeat("-", max(1, w))

	rows := h - 2
	start := scrollStart(len(values), cursor, rows)
	for row := 0; row < rows; row++ {
		idx := start + row
		lineAt := row + 2
		if idx >= len(values) {
			if row == 0 && len(values) == 0 {
				lines[lineAt] = "(none)"
			}
			continue
		}

		prefix := "  "
		if idx == cursor {
			if focused {
				prefix = "> "
			} else {
				prefix = "* "
			}
		}

		lines[lineAt] = compactText(prefix+strconv.Itoa(values[idx]), w)
	}

	return lines
}

func maybeYear(year int) string {
	if year <= 0 {
		return "n/a"
	}
	return strconv.Itoa(year)
}

func itemLabel(item api.MediaItem) string {
	kind := "MOV"
	if item.Type == "series" {
		kind = "TV"
	}
	label := fmt.Sprintf("[%s] %s", kind, item.Name)
	if item.Year > 0 {
		label += fmt.Sprintf(" (%d)", item.Year)
	}
	return label
}

func streamLabel(stream api.Stream) string {
	base := strings.TrimSpace(stream.Title)
	if base == "" {
		base = strings.TrimSpace(stream.Name)
	}
	if base == "" {
		base = "Torrent stream"
	}
	base = strings.ReplaceAll(base, "\n", " | ")
	return base
}

func sortedMapKeys(values map[int][]int) []int {
	keys := make([]int, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
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

func scrollStart(length int, cursor int, rows int) int {
	if rows <= 0 || length <= rows {
		return 0
	}

	cursor = clampCursor(cursor, length)
	start := cursor - rows/2
	if start < 0 {
		start = 0
	}
	maxStart := length - rows
	if start > maxStart {
		start = maxStart
	}

	return start
}

func clampCursor(index int, length int) int {
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

func padRight(input string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(input) >= width {
		return input[:width]
	}
	return input + strings.Repeat(" ", width-len(input))
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
