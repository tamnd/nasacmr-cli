package nasacmr

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in nasacmr_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "nasacmr" {
		t.Errorf("Scheme = %q, want nasacmr", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "nasacmr" {
		t.Errorf("Identity.Binary = %q, want nasacmr", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"C2021957657-LPCLOUD", "collection", "C2021957657-LPCLOUD"},
		{"/search/collections", "collection", "search/collections"},
		{"https://" + Host + "/search/granules.json", "collection", "search/granules.json"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("collection", "C2021957657-LPCLOUD")
	want := "https://" + Host + "/search/collections.json?concept_id=C2021957657-LPCLOUD"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}

	got2, err2 := Domain{}.Locate("granule", "G2021957800-LPCLOUD")
	want2 := "https://" + Host + "/search/granules.json?concept_id=G2021957800-LPCLOUD"
	if err2 != nil || got2 != want2 {
		t.Errorf("Locate(granule) = (%q, %v), want (%q, nil)", got2, err2, want2)
	}
}

// TestHostWiring mounts the driver in a kit Host (the runtime ant drives) and
// checks the round trip: a record mints to its URI, its body is readable, and a
// bare id resolves back to the same URI. The init in domain.go registers the
// domain, so kit.Open finds it.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	c := &Collection{
		ID:       "C2021957657-LPCLOUD",
		Title:    "HLS Landsat Operational Land Imager Surface Reflectance",
		Abstract: "The Harmonized Landsat and Sentinel-2 (HLS) project.",
	}
	u, err := h.Mint(c)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "nasacmr://collection/C2021957657-LPCLOUD"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	if body, ok := h.Body(c); !ok || body == "" {
		t.Errorf("Body = (%q, %v), want non-empty", body, ok)
	}

	got, err := h.ResolveOn("nasacmr", "MOD02QKM")
	if err != nil || got.String() != "nasacmr://collection/MOD02QKM" {
		t.Errorf("ResolveOn = (%q, %v), want nasacmr://collection/MOD02QKM", got.String(), err)
	}
}
