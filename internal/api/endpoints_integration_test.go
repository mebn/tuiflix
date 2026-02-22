package api

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func TestLiveCinemetaAndTorrentioEndpoints(t *testing.T) {
	requireLiveTests(t)

	client := NewClient(readRealDebridToken())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	movies, shows, err := client.FetchPopular(ctx)
	if err != nil {
		t.Fatalf("FetchPopular failed: %v", err)
	}
	if len(movies) == 0 {
		t.Fatal("FetchPopular returned no movies")
	}
	if len(shows) == 0 {
		t.Fatal("FetchPopular returned no shows")
	}

	results, err := client.Search(ctx, "matrix")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}

	episodes, err := client.FetchSeriesEpisodes(ctx, "tt0944947")
	if err != nil {
		t.Fatalf("FetchSeriesEpisodes failed: %v", err)
	}
	if len(episodes) == 0 {
		t.Fatal("FetchSeriesEpisodes returned no seasons")
	}

	streams, err := client.FetchStreams(ctx, MediaItem{ID: "tt0816692", Type: "movie"}, 1, 1)
	if err != nil {
		t.Fatalf("FetchStreams failed: %v", err)
	}
	t.Logf("Torrentio streams returned: %d", len(streams))
}

func TestLiveRealDebridEndpoint(t *testing.T) {
	requireLiveTests(t)
	token := readRealDebridToken()
	if token == "" {
		t.Skip("REALDEBRID token not set")
	}

	client := NewClient(token)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var payload struct {
		Username string `json:"username"`
	}
	if err := client.rd.getJSON(ctx, "/user", &payload); err != nil {
		t.Fatalf("Real-Debrid /user failed: %v", err)
	}
	if payload.Username == "" {
		t.Fatal("Real-Debrid /user returned empty username")
	}
	if !client.RealDebridEnabled() {
		t.Fatal("Real-Debrid expected to be enabled")
	}
}

func requireLiveTests(t *testing.T) {
	t.Helper()
	if os.Getenv("TUIFLIX_LIVE_TESTS") != "1" {
		t.Skip("set TUIFLIX_LIVE_TESTS=1 to run live endpoint tests")
	}
}

func readRealDebridToken() string {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../../.env")
	return os.Getenv("REALDEBRID")
}
