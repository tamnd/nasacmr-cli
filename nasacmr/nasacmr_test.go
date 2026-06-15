package nasacmr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockFeed builds a JSON response body in CMR feed format.
func mockFeed(entries []any) []byte {
	raws := make([]json.RawMessage, 0, len(entries))
	for _, e := range entries {
		b, _ := json.Marshal(e)
		raws = append(raws, b)
	}
	body := wireResp{Feed: wireFeed{Entry: raws}}
	b, _ := json.Marshal(body)
	return b
}

func testClient(baseURL string) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = baseURL
	cfg.Rate = 0 // no pacing in tests
	return NewClient(cfg)
}

func TestSearchCollections(t *testing.T) {
	colls := []wireCollection{
		{ID: "C1234-LPCLOUD", Title: "MODIS Land Cover", ShortName: "MCD12Q1", VersionID: "006", DataCenter: "LPCLOUD"},
		{ID: "C5678-NSIDC", Title: "Arctic Sea Ice Extent", ShortName: "NSIDC-0051", VersionID: "002", DataCenter: "NSIDC_ECS"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CMR-Hits", "54544")
		w.Header().Set("Content-Type", "application/json")
		entries := make([]any, len(colls))
		for i, c := range colls {
			entries[i] = c
		}
		w.Write(mockFeed(entries))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	results, total, err := c.SearchCollections(context.Background(), "land cover", "", 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 54544 {
		t.Errorf("total = %d, want 54544", total)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Title != "MODIS Land Cover" {
		t.Errorf("results[0].Title = %q, want MODIS Land Cover", results[0].Title)
	}
	if results[1].Title != "Arctic Sea Ice Extent" {
		t.Errorf("results[1].Title = %q, want Arctic Sea Ice Extent", results[1].Title)
	}
}

func TestGetCollection(t *testing.T) {
	col := wireCollection{
		ID:         "C2826848343-LPCLOUD",
		Title:      "Airborne Hyperspectral Imagery",
		ShortName:  "AHI-L1B",
		VersionID:  "001",
		DataCenter: "LPCLOUD",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CMR-Hits", "1")
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockFeed([]any{col}))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	result, err := c.GetCollection(context.Background(), "C2826848343-LPCLOUD")
	if err != nil {
		t.Fatal(err)
	}
	if result.ShortName != "AHI-L1B" {
		t.Errorf("ShortName = %q, want AHI-L1B", result.ShortName)
	}
	if result.ID != "C2826848343-LPCLOUD" {
		t.Errorf("ID = %q, want C2826848343-LPCLOUD", result.ID)
	}
}

func TestSearchGranules(t *testing.T) {
	granules := []wireGranule{
		{ID: "G1001-LPCLOUD", Title: "MOD13Q1.A2024001", TimeStart: "2024-01-01T00:00:00Z", TimeEnd: "2024-01-17T00:00:00Z", DataCenter: "LPCLOUD"},
		{ID: "G1002-LPCLOUD", Title: "MOD13Q1.A2024017", TimeStart: "2024-01-17T00:00:00Z", TimeEnd: "2024-02-02T00:00:00Z", DataCenter: "LPCLOUD"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CMR-Hits", "374000000")
		w.Header().Set("Content-Type", "application/json")
		entries := make([]any, len(granules))
		for i, g := range granules {
			entries[i] = g
		}
		w.Write(mockFeed(entries))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	results, total, err := c.SearchGranules(context.Background(), "MOD13Q1", "006", "2024-01-01,2024-02-01", 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 374000000 {
		t.Errorf("total = %d, want 374000000", total)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].TimeStart != "2024-01-01T00:00:00Z" {
		t.Errorf("results[0].TimeStart = %q, want 2024-01-01T00:00:00Z", results[0].TimeStart)
	}
	if results[1].TimeStart != "2024-01-17T00:00:00Z" {
		t.Errorf("results[1].TimeStart = %q, want 2024-01-17T00:00:00Z", results[1].TimeStart)
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	col := wireCollection{ID: "C999-PROVIDER", Title: "Recovered Collection", ShortName: "REC"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("CMR-Hits", "1")
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockFeed([]any{col}))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 3
	c := NewClient(cfg)

	start := time.Now()
	results, total, err := c.SearchCollections(context.Background(), "test", "", 10, 1)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 || results[0].ID != "C999-PROVIDER" {
		t.Errorf("unexpected results: %+v", results)
	}
	// backoff for attempt=1 is 500ms, so total must be >= 500ms
	if elapsed < 500*time.Millisecond {
		t.Errorf("retries did not back off: elapsed %v", elapsed)
	}
}
