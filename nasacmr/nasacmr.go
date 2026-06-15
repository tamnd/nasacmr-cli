// Package nasacmr is the library behind the nasacmr command line:
// the HTTP client, request shaping, and the typed data models for NASA's
// Common Metadata Repository (CMR), which indexes Earth science data
// collections and granules.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package nasacmr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to CMR.
const DefaultUserAgent = "nasacmr-cli/0.1.0"

// Host is the CMR hostname this client talks to, and the host the
// URI driver in domain.go claims.
const Host = "cmr.earthdata.nasa.gov"

// BaseURL is the CMR search endpoint every request is built from.
const BaseURL = "https://cmr.earthdata.nasa.gov/search"

// Config holds the runtime settings for the CMR client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to CMR over HTTPS.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client using the given Config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) pace() {
	if c.cfg.Rate > 0 {
		if since := time.Since(c.last); since < c.cfg.Rate {
			time.Sleep(c.cfg.Rate - since)
		}
	}
	c.last = time.Now()
}

// searchResult carries the parsed results from a CMR search response.
// Hits comes from the CMR-Hits response header, not the JSON body.
type searchResult struct {
	Hits    int
	Entries []json.RawMessage
}

// get issues a GET request to rawURL, reads the CMR-Hits header, and decodes
// the JSON body feed entries into a searchResult.
func (c *Client) get(ctx context.Context, rawURL string) (*searchResult, error) {
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			d := time.Duration(attempt) * 500 * time.Millisecond
			if d > 5*time.Second {
				d = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
		c.pace()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			if attempt < c.cfg.Retries {
				continue
			}
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < c.cfg.Retries {
				continue
			}
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		// Read the total hit count from the CMR-Hits response header.
		hits, _ := strconv.Atoi(resp.Header.Get("CMR-Hits"))

		var body wireResp
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return nil, err
		}
		return &searchResult{Hits: hits, Entries: body.Feed.Entry}, nil
	}
	return nil, fmt.Errorf("all retries exhausted")
}

// --- wire types (unexported) ---

type wireCollection struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ShortName    string `json:"short_name"`
	VersionID    string `json:"version_id"`
	DataCenter   string `json:"data_center"`
	Summary      string `json:"summary"`
	TimeStart    string `json:"time_start"`
	TimeEnd      string `json:"time_end"`
	CloudHosted  bool   `json:"cloud_hosted"`
	OnlineAccess bool   `json:"online_access_flag"`
}

type wireGranule struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	TimeStart    string  `json:"time_start"`
	TimeEnd      string  `json:"time_end"`
	DataCenter   string  `json:"data_center"`
	DayNightFlag string  `json:"day_night_flag"`
	CloudCover   float64 `json:"cloud_cover"`
	GranuleSize  float64 `json:"granule_size"`
	OnlineAccess bool    `json:"online_access_flag"`
}

type wireFeed struct {
	Entry []json.RawMessage `json:"entry"`
}

type wireResp struct {
	Feed wireFeed `json:"feed"`
}

// --- public types ---

// Collection is a single NASA CMR collection (dataset) record.
type Collection struct {
	ID          string `json:"id"           kit:"id"`
	Title       string `json:"title"`
	ShortName   string `json:"short_name"`
	Version     string `json:"version"`
	DataCenter  string `json:"data_center"`
	Summary     string `json:"summary"`
	TimeStart   string `json:"time_start"`
	TimeEnd     string `json:"time_end"`
	CloudHosted bool   `json:"cloud_hosted"`
}

// Granule is a single NASA CMR granule (file-level) record.
type Granule struct {
	ID         string  `json:"id"          kit:"id"`
	Title      string  `json:"title"`
	TimeStart  string  `json:"time_start"`
	TimeEnd    string  `json:"time_end"`
	DataCenter string  `json:"data_center"`
	DayNight   string  `json:"day_night"`
	CloudCover float64 `json:"cloud_cover"`
	SizeMB     float64 `json:"size_mb"`
	Online     bool    `json:"online"`
}

func toCollection(w wireCollection) Collection {
	return Collection{
		ID:          w.ID,
		Title:       w.Title,
		ShortName:   w.ShortName,
		Version:     w.VersionID,
		DataCenter:  w.DataCenter,
		Summary:     w.Summary,
		TimeStart:   w.TimeStart,
		TimeEnd:     w.TimeEnd,
		CloudHosted: w.CloudHosted,
	}
}

func toGranule(w wireGranule) Granule {
	return Granule{
		ID:         w.ID,
		Title:      w.Title,
		TimeStart:  w.TimeStart,
		TimeEnd:    w.TimeEnd,
		DataCenter: w.DataCenter,
		DayNight:   w.DayNightFlag,
		CloudCover: w.CloudCover,
		SizeMB:     w.GranuleSize,
		Online:     w.OnlineAccess,
	}
}

// SearchCollections searches CMR for collections matching keyword.
// It returns the matching collections, the total hit count from CMR-Hits, and any error.
func (c *Client) SearchCollections(ctx context.Context, keyword, provider string, limit, page int) ([]Collection, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if page <= 0 {
		page = 1
	}
	params := url.Values{}
	params.Set("page_size", strconv.Itoa(limit))
	params.Set("page_num", strconv.Itoa(page))
	if keyword != "" {
		params.Set("keyword", keyword)
	}
	if provider != "" {
		params.Set("provider", provider)
	}
	rawURL := c.cfg.BaseURL + "/collections.json?" + params.Encode()

	res, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, 0, err
	}

	out := make([]Collection, 0, len(res.Entries))
	for _, raw := range res.Entries {
		var w wireCollection
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, 0, err
		}
		out = append(out, toCollection(w))
	}
	return out, res.Hits, nil
}

// GetCollection fetches a single CMR collection by its concept ID
// (e.g. "C2826848343-LPCLOUD").
func (c *Client) GetCollection(ctx context.Context, conceptID string) (*Collection, error) {
	params := url.Values{}
	params.Set("concept_id", conceptID)
	params.Set("page_size", "1")
	rawURL := c.cfg.BaseURL + "/collections.json?" + params.Encode()

	res, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 0 {
		return nil, fmt.Errorf("collection %q not found", conceptID)
	}
	var w wireCollection
	if err := json.Unmarshal(res.Entries[0], &w); err != nil {
		return nil, err
	}
	col := toCollection(w)
	return &col, nil
}

// SearchGranules lists granules for the given short_name.
// temporal is an optional "start,end" string (e.g. "2024-01-01,2024-02-01").
// It returns the matching granules, the total hit count from CMR-Hits, and any error.
func (c *Client) SearchGranules(ctx context.Context, shortName, version, temporal string, limit, page int) ([]Granule, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if page <= 0 {
		page = 1
	}
	params := url.Values{}
	params.Set("page_size", strconv.Itoa(limit))
	params.Set("page_num", strconv.Itoa(page))
	params.Set("short_name", shortName)
	if version != "" {
		params.Set("version", version)
	}
	if temporal != "" {
		parts := strings.SplitN(temporal, ",", 2)
		if len(parts) == 2 {
			params.Set("temporal[]", parts[0]+","+parts[1])
		}
	}
	rawURL := c.cfg.BaseURL + "/granules.json?" + params.Encode()

	res, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, 0, err
	}

	out := make([]Granule, 0, len(res.Entries))
	for _, raw := range res.Entries {
		var w wireGranule
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, 0, err
		}
		out = append(out, toGranule(w))
	}
	return out, res.Hits, nil
}
