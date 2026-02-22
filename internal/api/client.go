package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cinemetaBase    = "https://v3-cinemeta.strem.io"
	torrentioBase   = "https://torrentio.strem.fun"
	realDebridBase  = "https://api.real-debrid.com/rest/1.0"
	defaultHTTPTime = 20 * time.Second
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
	http *http.Client
	rd   *realDebrid
}

func NewClient(rdToken string) *Client {
	return &Client{
		http: &http.Client{Timeout: defaultHTTPTime},
		rd: &realDebrid{
			token: strings.TrimSpace(rdToken),
			http:  &http.Client{Timeout: 45 * time.Second},
		},
	}
}

func (c *Client) RealDebridEnabled() bool {
	return c.rd.enabled()
}

func (c *Client) FetchPopular(ctx context.Context) ([]MediaItem, []MediaItem, error) {
	movies, err := c.fetchCatalog(ctx, "movie", "top")
	if err != nil {
		return nil, nil, err
	}

	shows, err := c.fetchCatalog(ctx, "series", "top")
	if err != nil {
		return nil, nil, err
	}

	return movies, shows, nil
}

func (c *Client) Search(ctx context.Context, query string) ([]MediaItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var (
		movies []MediaItem
		shows  []MediaItem
		errA   error
		errB   error
		wg     sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		movies, errA = c.fetchCatalog(ctx, "movie", "top/search="+url.PathEscape(query))
	}()
	go func() {
		defer wg.Done()
		shows, errB = c.fetchCatalog(ctx, "series", "top/search="+url.PathEscape(query))
	}()
	wg.Wait()

	if errA != nil {
		return nil, errA
	}
	if errB != nil {
		return nil, errB
	}

	results := append(movies, shows...)
	if len(results) > 60 {
		results = results[:60]
	}

	return results, nil
}

func (c *Client) FetchSeriesEpisodes(ctx context.Context, id string) (map[int][]int, error) {
	var payload struct {
		Meta struct {
			Videos []struct {
				Season  int `json:"season"`
				Episode int `json:"episode"`
			} `json:"videos"`
		} `json:"meta"`
	}

	endpoint := cinemetaBase + "/meta/series/" + url.PathEscape(id) + ".json"
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}

	bySeason := map[int][]int{}
	for _, video := range payload.Meta.Videos {
		if video.Season < 1 || video.Episode < 1 {
			continue
		}
		bySeason[video.Season] = append(bySeason[video.Season], video.Episode)
	}

	for season, episodes := range bySeason {
		sort.Ints(episodes)
		bySeason[season] = compactInts(episodes)
	}

	if len(bySeason) == 0 {
		bySeason[1] = []int{1}
	}

	return bySeason, nil
}

func (c *Client) FetchStreams(ctx context.Context, item MediaItem, season int, episode int) ([]Stream, error) {
	if item.ID == "" {
		return nil, errors.New("missing media id")
	}

	streamPath := ""
	switch item.Type {
	case "movie":
		streamPath = "/stream/movie/" + url.PathEscape(item.ID) + ".json"
	case "series":
		streamPath = fmt.Sprintf("/stream/series/%s:%d:%d.json", url.PathEscape(item.ID), season, episode)
	default:
		return nil, fmt.Errorf("unsupported media type: %s", item.Type)
	}

	var payload struct {
		Streams []struct {
			Name     string          `json:"name"`
			Title    string          `json:"title"`
			URL      string          `json:"url"`
			InfoHash string          `json:"infoHash"`
			FileIdx  json.RawMessage `json:"fileIdx"`
			Sources  []string        `json:"sources"`
		} `json:"streams"`
	}

	if err := c.getJSON(ctx, torrentioBase+streamPath, &payload); err != nil {
		return nil, err
	}

	streams := make([]Stream, 0, len(payload.Streams))
	for _, raw := range payload.Streams {
		idx := parseOptionalInt(raw.FileIdx)
		entry := Stream{
			Name:     strings.TrimSpace(raw.Name),
			Title:    strings.TrimSpace(raw.Title),
			URL:      strings.TrimSpace(raw.URL),
			InfoHash: strings.TrimSpace(raw.InfoHash),
			FileIdx:  idx,
			Sources:  raw.Sources,
		}

		if entry.URL == "" && entry.InfoHash == "" {
			continue
		}

		streams = append(streams, entry)
	}

	return streams, nil
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

func (c *Client) fetchCatalog(ctx context.Context, mediaType string, catalogPath string) ([]MediaItem, error) {
	var payload struct {
		Metas []struct {
			ID     string          `json:"id"`
			Name   string          `json:"name"`
			Type   string          `json:"type"`
			Year   json.RawMessage `json:"year"`
			Poster string          `json:"poster"`
		} `json:"metas"`
	}

	endpoint := cinemetaBase + path.Join("/catalog", mediaType, catalogPath) + ".json"
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}

	items := make([]MediaItem, 0, len(payload.Metas))
	for _, raw := range payload.Metas {
		itemType := raw.Type
		if itemType == "" {
			itemType = mediaType
		}

		item := MediaItem{
			ID:     raw.ID,
			Name:   raw.Name,
			Type:   itemType,
			Year:   parseYear(raw.Year),
			Poster: raw.Poster,
		}

		if item.ID == "" || item.Name == "" {
			continue
		}

		items = append(items, item)
	}

	return items, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(out); err != nil {
		return err
	}

	return nil
}

type realDebrid struct {
	token string
	http  *http.Client
}

func (r *realDebrid) enabled() bool {
	return strings.TrimSpace(r.token) != ""
}

func (r *realDebrid) resolveMagnet(ctx context.Context, magnet string, fileIdx *int) (string, error) {
	torrentID, err := r.addMagnet(ctx, magnet)
	if err != nil {
		return "", err
	}

	info, err := r.waitForTorrentInfo(ctx, torrentID)
	if err != nil {
		return "", err
	}

	selectedFileID := pickFileID(info, fileIdx)
	if selectedFileID == 0 {
		return "", errors.New("failed to pick torrent file")
	}

	if err := r.selectFiles(ctx, torrentID, []int{selectedFileID}); err != nil {
		return "", err
	}

	ready, err := r.waitForReadyLinks(ctx, torrentID)
	if err != nil {
		return "", err
	}

	if len(ready.Links) == 0 {
		return "", errors.New("torrent has no links")
	}

	return r.unrestrictLink(ctx, ready.Links[0])
}

func (r *realDebrid) addMagnet(ctx context.Context, magnet string) (string, error) {
	var payload struct {
		ID string `json:"id"`
	}

	values := url.Values{}
	values.Set("magnet", magnet)

	if err := r.postForm(ctx, "/torrents/addMagnet", values, &payload); err != nil {
		return "", err
	}

	if payload.ID == "" {
		return "", errors.New("real-debrid returned empty torrent id")
	}

	return payload.ID, nil
}

func (r *realDebrid) selectFiles(ctx context.Context, torrentID string, fileIDs []int) error {
	if len(fileIDs) == 0 {
		return errors.New("select files requires at least one file")
	}

	parts := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		parts = append(parts, strconv.Itoa(id))
	}

	values := url.Values{}
	values.Set("files", strings.Join(parts, ","))

	return r.postForm(ctx, "/torrents/selectFiles/"+url.PathEscape(torrentID), values, nil)
}

func (r *realDebrid) waitForTorrentInfo(ctx context.Context, torrentID string) (torrentInfo, error) {
	for i := 0; i < 8; i++ {
		info, err := r.torrentInfo(ctx, torrentID)
		if err != nil {
			return torrentInfo{}, err
		}

		if len(info.Files) > 0 {
			return info, nil
		}

		select {
		case <-ctx.Done():
			return torrentInfo{}, ctx.Err()
		case <-time.After(1200 * time.Millisecond):
		}
	}

	return torrentInfo{}, errors.New("torrent metadata did not become available")
}

func (r *realDebrid) waitForReadyLinks(ctx context.Context, torrentID string) (torrentInfo, error) {
	for i := 0; i < 30; i++ {
		info, err := r.torrentInfo(ctx, torrentID)
		if err != nil {
			return torrentInfo{}, err
		}

		if len(info.Links) > 0 {
			return info, nil
		}

		select {
		case <-ctx.Done():
			return torrentInfo{}, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}

	return torrentInfo{}, errors.New("timeout waiting for debrid links")
}

func (r *realDebrid) torrentInfo(ctx context.Context, torrentID string) (torrentInfo, error) {
	var payload torrentInfo
	err := r.getJSON(ctx, "/torrents/info/"+url.PathEscape(torrentID), &payload)
	if err != nil {
		return torrentInfo{}, err
	}
	return payload, nil
}

func (r *realDebrid) unrestrictLink(ctx context.Context, link string) (string, error) {
	var payload struct {
		Download string `json:"download"`
	}

	values := url.Values{}
	values.Set("link", link)

	if err := r.postForm(ctx, "/unrestrict/link", values, &payload); err != nil {
		return "", err
	}

	if payload.Download == "" {
		return "", errors.New("real-debrid returned empty download link")
	}

	return payload.Download, nil
}

func (r *realDebrid) getJSON(ctx context.Context, route string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realDebridBase+route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("real-debrid request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (r *realDebrid) postForm(ctx context.Context, route string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, realDebridBase+route, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("real-debrid request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

type torrentInfo struct {
	Status string `json:"status"`
	Files  []struct {
		ID    int    `json:"id"`
		Path  string `json:"path"`
		Bytes int64  `json:"bytes"`
	} `json:"files"`
	Links []string `json:"links"`
}

func buildMagnet(stream Stream) string {
	if stream.InfoHash == "" {
		return ""
	}

	magnet := "magnet:?xt=urn:btih:" + strings.ToLower(stream.InfoHash)
	seen := map[string]struct{}{}
	for _, source := range stream.Sources {
		if !strings.HasPrefix(source, "tracker:") {
			continue
		}
		tracker := strings.TrimSpace(strings.TrimPrefix(source, "tracker:"))
		if tracker == "" {
			continue
		}
		if _, ok := seen[tracker]; ok {
			continue
		}
		seen[tracker] = struct{}{}
		magnet += "&tr=" + url.QueryEscape(tracker)
	}

	return magnet
}

func pickFileID(info torrentInfo, fileIdx *int) int {
	if len(info.Files) == 0 {
		return 0
	}

	if fileIdx != nil {
		idx := *fileIdx
		if idx >= 0 && idx < len(info.Files) {
			return info.Files[idx].ID
		}
	}

	bestID := 0
	bestBytes := int64(-1)
	for _, file := range info.Files {
		if !isLikelyVideo(file.Path) {
			continue
		}
		if file.Bytes > bestBytes {
			bestID = file.ID
			bestBytes = file.Bytes
		}
	}

	if bestID != 0 {
		return bestID
	}

	return info.Files[0].ID
}

func isLikelyVideo(filePath string) bool {
	lower := strings.ToLower(filePath)
	for _, ext := range []string{".mkv", ".mp4", ".avi", ".mov", ".m4v", ".wmv", ".webm", ".ts"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func parseYear(raw json.RawMessage) int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}

	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			year, _ := strconv.Atoi(s)
			return year
		}
		return 0
	}

	var year int
	if err := json.Unmarshal(raw, &year); err == nil {
		return year
	}

	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int(f)
	}

	return 0
}

func parseOptionalInt(raw json.RawMessage) *int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}
		return &n
	}

	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return &n
	}

	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil
	}
	n = int(f)
	return &n
}

func compactInts(input []int) []int {
	if len(input) == 0 {
		return input
	}
	result := []int{input[0]}
	for i := 1; i < len(input); i++ {
		if input[i] == input[i-1] {
			continue
		}
		result = append(result, input[i])
	}
	return result
}
