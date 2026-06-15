// Package nasacmr is the library behind the nasacmr command line:
// the HTTP client, request shaping, and the typed data models for the
// NASA Common Metadata Repository (CMR) API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// No API key is required.
package nasacmr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Host is the CMR hostname this driver claims for URI routing.
const Host = "cmr.earthdata.nasa.gov"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Config holds all tunable parameters for the CMR client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults for the CMR client.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		UserAgent: "nasacmr-cli/0.1 (tamnd87@gmail.com)",
		Rate:      500 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to the NASA CMR API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string

	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with DefaultConfig settings.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// SearchCollections searches CMR collections by keyword, provider, platform, etc.
// Returns collections and the total hit count from the CMR-Hits header.
func (c *Client) SearchCollections(ctx context.Context, params CollectionParams) ([]*Collection, int, error) {
	u := c.buildURL("/search/collections.json", params.toQuery()...)
	body, headers, err := c.getWithHeaders(ctx, u)
	if err != nil {
		return nil, 0, err
	}

	var raw wireCollectionsResp
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, fmt.Errorf("collections decode: %w", err)
	}

	total, _ := strconv.Atoi(headers.Get("CMR-Hits"))

	out := make([]*Collection, 0, len(raw.Feed.Entry))
	for _, e := range raw.Feed.Entry {
		out = append(out, &Collection{
			ID:              e.ID,
			ShortName:       e.ShortName,
			Title:           e.Title,
			Abstract:        e.Abstract,
			Provider:        e.ArchiveCenter,
			ProcessingLevel: e.ProcessingLevel,
			Platforms:       e.Platforms,
			ScienceKeywords: e.ScienceKeywords,
			StartTime:       e.TimeStart,
			EndTime:         e.TimeEnd,
		})
	}
	return out, total, nil
}

// ListGranules lists granules for a collection by short name, concept ID, temporal range, etc.
// Returns granules and the total hit count from the CMR-Hits header.
func (c *Client) ListGranules(ctx context.Context, params GranuleParams) ([]*Granule, int, error) {
	u := c.buildURL("/search/granules.json", params.toQuery()...)
	body, headers, err := c.getWithHeaders(ctx, u)
	if err != nil {
		return nil, 0, err
	}

	var raw wireGranulesResp
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, fmt.Errorf("granules decode: %w", err)
	}

	total, _ := strconv.Atoi(headers.Get("CMR-Hits"))

	out := make([]*Granule, 0, len(raw.Feed.Entry))
	for _, e := range raw.Feed.Entry {
		var accessURLs []string
		for _, l := range e.Links {
			if strings.Contains(l.Rel, "data") || strings.Contains(l.Type, "data") ||
				l.Rel == "http://esipfed.org/ns/fedsearch/1.1/data#" {
				accessURLs = append(accessURLs, l.Href)
			}
		}
		out = append(out, &Granule{
			ID:               e.ID,
			Title:            e.Title,
			GranuleUR:        e.GranuleUR,
			Provider:         e.DataCenter,
			Size:             e.GranuleSizeMB,
			OnlineAccessURLs: accessURLs,
			StartTime:        e.TimeStart,
			EndTime:          e.TimeEnd,
		})
	}
	return out, total, nil
}

// buildURL assembles a full request URL with optional key=value pairs.
// Pairs with empty values are skipped.
func (c *Client) buildURL(path string, params ...string) string {
	base := c.BaseURL
	if base == "" {
		base = BaseURL
	}
	sb := strings.Builder{}
	sb.WriteString(base)
	sb.WriteString(path)
	sep := "?"
	for i := 0; i+1 < len(params); i += 2 {
		if params[i+1] != "" {
			sb.WriteString(sep)
			sb.WriteString(url.QueryEscape(params[i]))
			sb.WriteString("=")
			sb.WriteString(url.QueryEscape(params[i+1]))
			sep = "&"
		}
	}
	return sb.String()
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	body, _, err := c.getWithHeaders(ctx, rawURL)
	return body, err
}

// getWithHeaders fetches url and returns the response body and headers.
func (c *Client) getWithHeaders(ctx context.Context, rawURL string) ([]byte, http.Header, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, headers, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, headers, nil
		}
		lastErr = err
		if !retry {
			return nil, nil, err
		}
	}
	return nil, nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, headers http.Header, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, true, err
	}
	return b, resp.Header, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- Public data types ---

// Collection is a NASA CMR data collection record.
type Collection struct {
	ID              string   `json:"id"                        kit:"id"`
	ShortName       string   `json:"short_name,omitempty"`
	Title           string   `json:"title,omitempty"`
	Abstract        string   `json:"abstract,omitempty"        kit:"body"`
	Provider        string   `json:"provider,omitempty"`
	ProcessingLevel string   `json:"processing_level,omitempty"`
	Platforms       []string `json:"platforms,omitempty"`
	ScienceKeywords []string `json:"science_keywords,omitempty"`
	StartTime       string   `json:"start_time,omitempty"`
	EndTime         string   `json:"end_time,omitempty"`
}

// Granule is a NASA CMR granule (a single data file within a collection).
type Granule struct {
	ID               string   `json:"id"                         kit:"id"`
	Title            string   `json:"title,omitempty"`
	GranuleUR        string   `json:"granule_ur,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	Size             float64  `json:"size_mb,omitempty"`
	OnlineAccessURLs []string `json:"online_access_urls,omitempty"`
	StartTime        string   `json:"start_time,omitempty"`
	EndTime          string   `json:"end_time,omitempty"`
}

// CollectionParams holds search parameters for collection queries.
type CollectionParams struct {
	Keyword         string
	Provider        string
	Platform        string
	ShortName       string
	SpatialCoverage string
	PageSize        int
}

func (p CollectionParams) toQuery() []string {
	var q []string
	if p.Keyword != "" {
		q = append(q, "keyword", p.Keyword)
	}
	if p.Provider != "" {
		q = append(q, "provider", p.Provider)
	}
	if p.Platform != "" {
		q = append(q, "platform", p.Platform)
	}
	if p.ShortName != "" {
		q = append(q, "short_name", p.ShortName)
	}
	if p.SpatialCoverage != "" {
		q = append(q, "bounding_box", p.SpatialCoverage)
	}
	n := p.PageSize
	if n <= 0 {
		n = 10
	}
	q = append(q, "page_size", strconv.Itoa(n))
	return q
}

// GranuleParams holds search parameters for granule queries.
type GranuleParams struct {
	ShortName string
	ConceptID string
	Temporal  string
	Provider  string
	PageSize  int
}

func (p GranuleParams) toQuery() []string {
	var q []string
	if p.ShortName != "" {
		q = append(q, "short_name", p.ShortName)
	}
	if p.ConceptID != "" {
		q = append(q, "concept_id", p.ConceptID)
	}
	if p.Temporal != "" {
		q = append(q, "temporal", p.Temporal)
	}
	if p.Provider != "" {
		q = append(q, "provider", p.Provider)
	}
	n := p.PageSize
	if n <= 0 {
		n = 10
	}
	q = append(q, "page_size", strconv.Itoa(n))
	return q
}

// --- Wire types (internal, not exported) ---

type wireCollectionsResp struct {
	Feed struct {
		Entry []wireCollection `json:"entry"`
	} `json:"feed"`
}

type wireCollection struct {
	ID              string   `json:"id"`
	ShortName       string   `json:"short_name"`
	Title           string   `json:"title"`
	Abstract        string   `json:"summary"` // note: "summary" in CMR JSON
	Platforms       []string `json:"platforms"`
	ArchiveCenter   string   `json:"archive_center"`
	ProcessingLevel string   `json:"processing_level_id"`
	ScienceKeywords []string `json:"science_keywords"`
	TimeStart       string   `json:"time_start"`
	TimeEnd         string   `json:"time_end"`
}

type wireGranulesResp struct {
	Feed struct {
		Entry []wireGranule `json:"entry"`
	} `json:"feed"`
}

type wireGranule struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	GranuleUR     string  `json:"producer_granule_id"`
	DataCenter    string  `json:"data_center"`
	GranuleSizeMB float64 `json:"granule_size"`
	TimeStart     string  `json:"time_start"`
	TimeEnd       string  `json:"time_end"`
	Links         []struct {
		Href string `json:"href"`
		Rel  string `json:"rel"`
		Type string `json:"type"`
	} `json:"links"`
}
