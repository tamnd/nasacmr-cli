package nasacmr

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes nasacmr as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/nasacmr-cli/nasacmr"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// nasacmr:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone nasacmr binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the nasacmr driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "nasacmr",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "nasacmr",
			Short:  "A command line for the NASA Common Metadata Repository.",
			Long: `A command line for the NASA Common Metadata Repository (CMR).

nasacmr searches and lists Earth science data indexed by NASA — over 54,000
collections and millions of granules — over plain HTTPS, shapes it into clean
records, and prints output that pipes into the rest of your tools. No API key
required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/nasacmr-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// collection: fetch a single collection by concept ID (resolver op).
	kit.Handle(app, kit.OpMeta{
		Name: "collection", Group: "read", Single: true,
		Summary: "Fetch a NASA CMR collection by concept ID",
		URIType: "collection", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "concept ID or URL"}},
	}, getCollection)

	// collections: search NASA CMR collections by keyword, provider, or platform.
	kit.Handle(app, kit.OpMeta{
		Name:    "collections",
		Group:   "read",
		List:    true,
		Summary: "Search NASA CMR collections by keyword, provider, or platform",
		Args:    []kit.Arg{{Name: "keyword", Help: "search keyword"}},
	}, searchCollections)

	// granules: list granules for a collection by short name or concept ID.
	kit.Handle(app, kit.OpMeta{
		Name:    "granules",
		Group:   "read",
		List:    true,
		Summary: "List granules for a collection",
		Args:    []kit.Arg{{Name: "short-name", Help: "collection short name"}},
	}, listGranules)

	// search: full-text search across all NASA CMR collections.
	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		List:    true,
		Summary: "Full-text search across all NASA CMR collections",
		Args:    []kit.Arg{{Name: "keyword", Help: "search terms"}},
	}, searchAll)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
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
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- input structs ---

type collectionRef struct {
	Ref    string  `kit:"arg" help:"concept ID or URL"`
	Client *Client `kit:"inject"`
}

type collectionsInput struct {
	Keyword         string  `kit:"flag" name:"keyword"          help:"search keyword"`
	Provider        string  `kit:"flag" name:"provider"         help:"data provider ID (e.g. LAADS, LPCLOUD)"`
	Platform        string  `kit:"flag" name:"platform"         help:"platform name (e.g. Terra, Aqua)"`
	ShortName       string  `kit:"flag" name:"short-name"       help:"collection short name"`
	SpatialCoverage string  `kit:"flag" name:"spatial-coverage" help:"bounding box: W,S,E,N"`
	PageSize        int     `kit:"flag" name:"page-size"        help:"number of results (default 10)"`
	Limit           int     `kit:"flag,inherit"                  help:"max results"`
	Client          *Client `kit:"inject"`
}

type granulesInput struct {
	ShortName string  `kit:"flag" name:"short-name"  help:"collection short name (e.g. HLSL30)"`
	ConceptID string  `kit:"flag" name:"concept-id"  help:"collection concept ID (e.g. C2021957657-LPCLOUD)"`
	Temporal  string  `kit:"flag" name:"temporal"    help:"time range: 2024-01-01T00:00:00Z,2024-01-31T23:59:59Z"`
	Provider  string  `kit:"flag" name:"provider"    help:"data provider ID"`
	PageSize  int     `kit:"flag" name:"page-size"   help:"number of results (default 10)"`
	Limit     int     `kit:"flag,inherit"             help:"max results"`
	Client    *Client `kit:"inject"`
}

type searchInput struct {
	Keyword         string  `kit:"flag" name:"keyword"          help:"search terms"`
	SpatialCoverage string  `kit:"flag" name:"spatial-coverage" help:"bounding box: W,S,E,N"`
	PageSize        int     `kit:"flag" name:"page-size"        help:"number of results (default 10)"`
	Limit           int     `kit:"flag,inherit"                  help:"max results"`
	Client          *Client `kit:"inject"`
}

// --- handlers ---

func getCollection(ctx context.Context, in collectionRef, emit func(*Collection) error) error {
	ref := strings.TrimSpace(in.Ref)
	cols, _, err := in.Client.SearchCollections(ctx, CollectionParams{
		Keyword:  ref,
		PageSize: 1,
	})
	if err != nil {
		return mapErr(err)
	}
	if len(cols) == 0 {
		return fmt.Errorf("collection not found: %s", ref)
	}
	return emit(cols[0])
}

func searchCollections(ctx context.Context, in collectionsInput, emit func(*Collection) error) error {
	n := in.PageSize
	if n <= 0 && in.Limit > 0 {
		n = in.Limit
	}
	cols, _, err := in.Client.SearchCollections(ctx, CollectionParams{
		Keyword:         in.Keyword,
		Provider:        in.Provider,
		Platform:        in.Platform,
		ShortName:       in.ShortName,
		SpatialCoverage: in.SpatialCoverage,
		PageSize:        n,
	})
	if err != nil {
		return mapErr(err)
	}
	for _, c := range cols {
		if err := emit(c); err != nil {
			return err
		}
	}
	return nil
}

func listGranules(ctx context.Context, in granulesInput, emit func(*Granule) error) error {
	n := in.PageSize
	if n <= 0 && in.Limit > 0 {
		n = in.Limit
	}
	items, _, err := in.Client.ListGranules(ctx, GranuleParams{
		ShortName: in.ShortName,
		ConceptID: in.ConceptID,
		Temporal:  in.Temporal,
		Provider:  in.Provider,
		PageSize:  n,
	})
	if err != nil {
		return mapErr(err)
	}
	for _, g := range items {
		if err := emit(g); err != nil {
			return err
		}
	}
	return nil
}

func searchAll(ctx context.Context, in searchInput, emit func(*Collection) error) error {
	n := in.PageSize
	if n <= 0 && in.Limit > 0 {
		n = in.Limit
	}
	cols, _, err := in.Client.SearchCollections(ctx, CollectionParams{
		Keyword:         in.Keyword,
		SpatialCoverage: in.SpatialCoverage,
		PageSize:        n,
	})
	if err != nil {
		return mapErr(err)
	}
	for _, c := range cols {
		if err := emit(c); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver (URI driver) ---

// Classify turns any accepted input — a bare concept ID or a full CMR URL —
// into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = cmrPath(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized nasacmr reference: %q", input)
	}
	return "collection", id, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "collection":
		return "https://" + Host + "/search/collections.json?concept_id=" + id, nil
	case "granule":
		return "https://" + Host + "/search/granules.json?concept_id=" + id, nil
	default:
		return "", errs.Usage("nasacmr has no resource type %q", uriType)
	}
}

// --- helpers ---

// cmrPath turns any accepted input into the canonical resource id: the
// concept ID from a full URL on this host, or a bare concept ID.
func cmrPath(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return strings.Trim(u.Path, "/")
	}
	return strings.Trim(input, "/")
}

func mapErr(err error) error {
	return err
}
