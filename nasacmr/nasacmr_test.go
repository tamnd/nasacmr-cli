package nasacmr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchCollections(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/collections.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		kw := r.URL.Query().Get("keyword")
		if kw != "landsat" {
			t.Errorf("keyword = %q, want landsat", kw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("CMR-Hits", "54544")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"feed": map[string]interface{}{
				"entry": []map[string]interface{}{
					{
						"id":         "C123-LAADS",
						"short_name": "MOD02QKM",
						"title":      "MODIS/Terra Calibrated Radiances",
						"summary":    "Calibrated radiances in 250m resolution",
						"archive_center": "LAADS",
					},
				},
			},
		})
	}))
	defer ts.Close()

	c := NewClient()
	c.BaseURL = ts.URL
	c.Rate = 0

	cols, total, err := c.SearchCollections(context.Background(), CollectionParams{
		Keyword:  "landsat",
		PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 54544 {
		t.Errorf("total = %d, want 54544", total)
	}
	if len(cols) != 1 {
		t.Fatalf("len(cols) = %d, want 1", len(cols))
	}
	if cols[0].ID != "C123-LAADS" {
		t.Errorf("ID = %q, want C123-LAADS", cols[0].ID)
	}
	if cols[0].ShortName != "MOD02QKM" {
		t.Errorf("ShortName = %q, want MOD02QKM", cols[0].ShortName)
	}
	if cols[0].Abstract != "Calibrated radiances in 250m resolution" {
		t.Errorf("Abstract = %q", cols[0].Abstract)
	}
	if cols[0].Provider != "LAADS" {
		t.Errorf("Provider = %q, want LAADS", cols[0].Provider)
	}
}

func TestSearchCollectionsByProvider(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prov := r.URL.Query().Get("provider")
		if prov != "LAADS" {
			t.Errorf("provider = %q, want LAADS", prov)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("CMR-Hits", "200")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"feed": map[string]interface{}{
				"entry": []map[string]interface{}{
					{"id": "C456-LAADS", "short_name": "MOD04_L2", "title": "MODIS Aerosol", "summary": "Aerosol data"},
				},
			},
		})
	}))
	defer ts.Close()

	c := NewClient()
	c.BaseURL = ts.URL
	c.Rate = 0

	cols, total, err := c.SearchCollections(context.Background(), CollectionParams{
		Provider: "LAADS",
		PageSize: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 200 {
		t.Errorf("total = %d, want 200", total)
	}
	if len(cols) != 1 {
		t.Fatalf("len(cols) = %d, want 1", len(cols))
	}
	if cols[0].ID != "C456-LAADS" {
		t.Errorf("ID = %q, want C456-LAADS", cols[0].ID)
	}
}

func TestListGranules(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/granules.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		sn := r.URL.Query().Get("short_name")
		if sn != "HLSL30" {
			t.Errorf("short_name = %q, want HLSL30", sn)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("CMR-Hits", "12345")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"feed": map[string]interface{}{
				"entry": []map[string]interface{}{
					{
						"id":                   "G2021957800-LPCLOUD",
						"title":                "HLS.L30.T10UDV.2024001T185318.v2.0",
						"producer_granule_id":  "HLS.L30.T10UDV.2024001T185318.v2.0",
						"data_center":          "LPCLOUD",
						"granule_size":         189.5,
						"time_start":           "2024-01-01T18:53:18Z",
						"time_end":             "2024-01-01T18:53:36Z",
						"links": []map[string]interface{}{
							{
								"href": "https://data.lpdaac.earthdatacloud.nasa.gov/lp-prod-protected/HLSL30.020/HLS.L30.T10UDV.2024001T185318.v2.0.B02.tif",
								"rel":  "http://esipfed.org/ns/fedsearch/1.1/data#",
								"type": "image/tiff",
							},
						},
					},
				},
			},
		})
	}))
	defer ts.Close()

	c := NewClient()
	c.BaseURL = ts.URL
	c.Rate = 0

	grans, total, err := c.ListGranules(context.Background(), GranuleParams{
		ShortName: "HLSL30",
		PageSize:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 12345 {
		t.Errorf("total = %d, want 12345", total)
	}
	if len(grans) != 1 {
		t.Fatalf("len(grans) = %d, want 1", len(grans))
	}
	g := grans[0]
	if g.ID != "G2021957800-LPCLOUD" {
		t.Errorf("ID = %q, want G2021957800-LPCLOUD", g.ID)
	}
	if g.Provider != "LPCLOUD" {
		t.Errorf("Provider = %q, want LPCLOUD", g.Provider)
	}
	if g.Size != 189.5 {
		t.Errorf("Size = %v, want 189.5", g.Size)
	}
	if len(g.OnlineAccessURLs) != 1 {
		t.Errorf("OnlineAccessURLs len = %d, want 1", len(g.OnlineAccessURLs))
	}
}

func TestListGranulesWithTemporal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		temp := r.URL.Query().Get("temporal")
		if temp == "" {
			t.Error("temporal param missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("CMR-Hits", "3")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"feed": map[string]interface{}{
				"entry": []map[string]interface{}{
					{"id": "G001", "title": "granule1"},
					{"id": "G002", "title": "granule2"},
				},
			},
		})
	}))
	defer ts.Close()

	c := NewClient()
	c.BaseURL = ts.URL
	c.Rate = 0

	grans, total, err := c.ListGranules(context.Background(), GranuleParams{
		ShortName: "HLSL30",
		Temporal:  "2024-01-01T00:00:00Z,2024-01-31T23:59:59Z",
		PageSize:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(grans) != 2 {
		t.Errorf("len(grans) = %d, want 2", len(grans))
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseURL != BaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, BaseURL)
	}
	if cfg.Rate != 500*time.Millisecond {
		t.Errorf("Rate = %v, want 500ms", cfg.Rate)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.Retries != 3 {
		t.Errorf("Retries = %d, want 3", cfg.Retries)
	}
	if cfg.UserAgent == "" {
		t.Error("UserAgent must not be empty")
	}
}
