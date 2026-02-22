package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	defaultHTTPTime = 20 * time.Second
	appUserAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)

type MediaItem struct {
	ID     string
	Name   string
	Type   string
	Year   int
	Poster string
}

type Stream struct {
	Name     string
	Title    string
	URL      string
	InfoHash string
	FileIdx  *int
	Sources  []string
}

type Client struct {
	cinemeta  *cinemetaService
	torrentio *torrentioService
	rd        *realDebridService
}

func NewClient(rdToken string) *Client {
	httpClient := &http.Client{Timeout: defaultHTTPTime}

	return &Client{
		cinemeta:  newCinemetaService(httpClient),
		torrentio: newTorrentioService(httpClient),
		rd:        newRealDebridService(strings.TrimSpace(rdToken)),
	}
}

func (c *Client) RealDebridEnabled() bool {
	return c.rd.enabled()
}

func (c *Client) FetchPopular(ctx context.Context) ([]MediaItem, []MediaItem, error) {
	movies, err := c.cinemeta.fetchCatalog(ctx, "movie", "top")
	if err != nil {
		return nil, nil, err
	}

	shows, err := c.cinemeta.fetchCatalog(ctx, "series", "top")
	if err != nil {
		return nil, nil, err
	}

	return movies, shows, nil
}

func (c *Client) Search(ctx context.Context, query string) ([]MediaItem, error) {
	return c.cinemeta.search(ctx, query)
}

func (c *Client) FetchSeriesEpisodes(ctx context.Context, id string) (map[int][]int, error) {
	return c.cinemeta.fetchSeriesEpisodes(ctx, id)
}

func (c *Client) FetchStreams(ctx context.Context, item MediaItem, season int, episode int) ([]Stream, error) {
	return c.torrentio.fetchStreams(ctx, item, season, episode)
}

func (c *Client) ResolvePlayableURL(ctx context.Context, stream Stream) (string, error) {
	if stream.URL != "" && strings.HasPrefix(strings.ToLower(stream.URL), "http") {
		if !c.rd.enabled() {
			return stream.URL, nil
		}

		link, err := c.rd.unrestrictLink(ctx, stream.URL)
		if err != nil {
			return stream.URL, nil
		}
		return link, nil
	}

	magnet := stream.URL
	if !strings.HasPrefix(strings.ToLower(magnet), "magnet:") {
		magnet = buildMagnet(stream)
	}

	if magnet == "" {
		return "", errors.New("stream does not include a playable URL")
	}

	if !c.rd.enabled() {
		return magnet, nil
	}

	link, err := c.rd.resolveMagnet(ctx, magnet, stream.FileIdx)
	if err != nil {
		return magnet, nil
	}

	return link, nil
}
