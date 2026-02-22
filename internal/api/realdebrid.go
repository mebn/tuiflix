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
	"time"
)

const realDebridBase = "https://api.real-debrid.com/rest/1.0"

type realDebridService struct {
	token string
	http  *http.Client
}

func newRealDebridService(token string) *realDebridService {
	return &realDebridService{
		token: strings.TrimSpace(token),
		http:  &http.Client{Timeout: 45 * time.Second},
	}
}

func (r *realDebridService) enabled() bool {
	return strings.TrimSpace(r.token) != ""
}

func (r *realDebridService) resolveMagnet(ctx context.Context, magnet string, fileIdx *int) (string, error) {
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

func (r *realDebridService) addMagnet(ctx context.Context, magnet string) (string, error) {
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

func (r *realDebridService) selectFiles(ctx context.Context, torrentID string, fileIDs []int) error {
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

func (r *realDebridService) waitForTorrentInfo(ctx context.Context, torrentID string) (torrentInfo, error) {
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

func (r *realDebridService) waitForReadyLinks(ctx context.Context, torrentID string) (torrentInfo, error) {
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

func (r *realDebridService) torrentInfo(ctx context.Context, torrentID string) (torrentInfo, error) {
	var payload torrentInfo
	err := r.getJSON(ctx, "/torrents/info/"+url.PathEscape(torrentID), &payload)
	if err != nil {
		return torrentInfo{}, err
	}
	return payload, nil
}

func (r *realDebridService) unrestrictLink(ctx context.Context, link string) (string, error) {
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

func (r *realDebridService) getJSON(ctx context.Context, route string, out any) error {
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

func (r *realDebridService) postForm(ctx context.Context, route string, values url.Values, out any) error {
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
		_, _ = io.Copy(io.Discard, resp.Body)
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
