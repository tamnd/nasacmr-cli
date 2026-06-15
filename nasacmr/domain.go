package nasacmr

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes NASA CMR as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/nasacmr-cli/nasacmr"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then
// dereferences nasacmr:// URIs by routing to the operations Register installs.
// The same Domain also builds the standalone nasacmr binary (see cli.NewApp),
// so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the NASA CMR driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "nasacmr",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "nasacmr",
			Short:  "A command line for NASA's Common Metadata Repository (CMR).",
			Long: `A command line for NASA's Common Metadata Repository (CMR).

nasacmr reads public Earth science data from NASA CMR, which indexes 54,000+
collections and hundreds of millions of granules. No API key required. Output
pipes cleanly into the rest of your tools.`,
			Site: "https://" + Host,
			Repo: "https://github.com/tamnd/nasacmr-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search collections by keyword",
		Args:    []kit.Arg{{Name: "keyword", Help: "search keyword"}}},
		searchCollections)

	kit.Handle(app, kit.OpMeta{Name: "collection", Group: "read", Single: true,
		Summary:  "Get a collection by concept ID",
		URIType:  "collection",
		Resolver: true,
		Args:     []kit.Arg{{Name: "id", Help: "concept ID (e.g. C2826848343-LPCLOUD)"}}},
		getCollection)

	kit.Handle(app, kit.OpMeta{Name: "granules", Group: "read", List: true,
		Summary: "List granules for a collection short name",
		Args:    []kit.Arg{{Name: "short-name", Help: "collection short name (e.g. MOD13Q1)"}}},
		searchGranules)
}

// newClient builds the CMR client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- inputs ---

type searchInput struct {
	Keyword  string  `kit:"arg"          help:"search keyword"`
	Provider string  `kit:"flag"         help:"filter by provider (e.g. LPCLOUD)"`
	Limit    int     `kit:"flag,inherit" help:"max results"`
	Client   *Client `kit:"inject"`
}

type collectionInput struct {
	ID     string  `kit:"arg"    help:"concept ID (e.g. C2826848343-LPCLOUD)"`
	Client *Client `kit:"inject"`
}

type granulesInput struct {
	ShortName string  `kit:"arg"          help:"collection short name (e.g. MOD13Q1)"`
	Version   string  `kit:"flag"         help:"collection version"`
	Temporal  string  `kit:"flag"         help:"time range as start,end (e.g. 2024-01-01,2024-02-01)"`
	Limit     int     `kit:"flag,inherit" help:"max results"`
	Client    *Client `kit:"inject"`
}

// --- handlers ---

func searchCollections(ctx context.Context, in searchInput, emit func(*Collection) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	cols, _, err := in.Client.SearchCollections(ctx, in.Keyword, in.Provider, limit, 1)
	if err != nil {
		return err
	}
	for i := range cols {
		if err := emit(&cols[i]); err != nil {
			return err
		}
	}
	return nil
}

func getCollection(ctx context.Context, in collectionInput, emit func(*Collection) error) error {
	col, err := in.Client.GetCollection(ctx, in.ID)
	if err != nil {
		return err
	}
	return emit(col)
}

func searchGranules(ctx context.Context, in granulesInput, emit func(*Granule) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	granules, _, err := in.Client.SearchGranules(ctx, in.ShortName, in.Version, in.Temporal, limit, 1)
	if err != nil {
		return err
	}
	for i := range granules {
		if err := emit(&granules[i]); err != nil {
			return err
		}
	}
	return nil
}

// Classify turns any accepted input into the canonical (type, id).
// A concept ID starts with "C" and contains "-"; anything else is treated
// as a collection keyword search reference.
func (Domain) Classify(input string) (uriType, id string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", errs.Usage("empty NASA CMR reference")
	}
	if strings.HasPrefix(s, "C") && strings.Contains(s, "-") {
		return "collection", s, nil
	}
	return "collection", s, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "collection":
		return fmt.Sprintf("https://search.earthdata.nasa.gov/search/granules?p=%s", id), nil
	case "granule":
		return fmt.Sprintf("https://%s/search/concepts/%s", Host, id), nil
	default:
		return "", errs.Usage("nasacmr has no resource type %q", uriType)
	}
}
