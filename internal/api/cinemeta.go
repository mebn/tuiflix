package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const cinemetaBase = "https://v3-cinemeta.strem.io"

type cinemetaService struct {
	http *http.Client
}

func newCinemetaService(httpClient *http.Client) *cinemetaService {
	return &cinemetaService{http: httpClient}
}

func (s *cinemetaService) search(ctx context.Context, query string) ([]MediaItem, error) {
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
		movies, errA = s.fetchCatalog(ctx, "movie", "top/search="+url.PathEscape(query))
	}()
	go func() {
		defer wg.Done()
		shows, errB = s.fetchCatalog(ctx, "series", "top/search="+url.PathEscape(query))
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

func (s *cinemetaService) fetchSeriesEpisodes(ctx context.Context, id string) (map[int][]int, error) {
	var payload struct {
		Meta struct {
			Videos []struct {
				Season  int `json:"season"`
				Episode int `json:"episode"`
			} `json:"videos"`
		} `json:"meta"`
	}

	endpoint := cinemetaBase + "/meta/series/" + url.PathEscape(id) + ".json"
	if err := s.getJSON(ctx, endpoint, &payload); err != nil {
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

func (s *cinemetaService) fetchCatalog(ctx context.Context, mediaType string, catalogPath string) ([]MediaItem, error) {
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
	if err := s.getJSON(ctx, endpoint, &payload); err != nil {
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

func (s *cinemetaService) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", appUserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.http.Do(req)
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
