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
	"strconv"
	"strings"
)

const torrentioBase = "https://torrentio.strem.fun"

type torrentioService struct {
	http *http.Client
}

func newTorrentioService(httpClient *http.Client) *torrentioService {
	return &torrentioService{http: httpClient}
}

func (s *torrentioService) fetchStreams(ctx context.Context, item MediaItem, season int, episode int) ([]Stream, error) {
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

	if err := s.getJSON(ctx, torrentioBase+streamPath, &payload); err != nil {
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

func (s *torrentioService) getJSON(ctx context.Context, endpoint string, out any) error {
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
