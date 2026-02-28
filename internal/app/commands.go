package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiflix/internal/api"
	"tuiflix/internal/player"
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
