package nasacmr

import (
	"strings"
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in nasacmr_test.go.

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
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"C2826848343-LPCLOUD", "collection", "C2826848343-LPCLOUD"},
		{"MOD13Q1", "collection", "MOD13Q1"},
		{"land surface temperature", "collection", "land surface temperature"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) error: %v", tc.in, err)
			continue
		}
		if typ != tc.typ {
			t.Errorf("Classify(%q) type = %q, want %q", tc.in, typ, tc.typ)
		}
		if id != tc.id {
			t.Errorf("Classify(%q) id = %q, want %q", tc.in, id, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") expected error, got nil")
	}
	_, _, err = Domain{}.Classify("   ")
	if err == nil {
		t.Error("Classify(\"   \") expected error, got nil")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("collection", "C2826848343-LPCLOUD")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	if !strings.Contains(got, "C2826848343-LPCLOUD") {
		t.Errorf("Locate = %q, expected to contain concept ID", got)
	}
	if !strings.Contains(got, "search.earthdata.nasa.gov") {
		t.Errorf("Locate = %q, expected search.earthdata.nasa.gov", got)
	}
}

func TestLocateGranule(t *testing.T) {
	got, err := Domain{}.Locate("granule", "G1234567890-LPCLOUD")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	if !strings.Contains(got, "G1234567890-LPCLOUD") {
		t.Errorf("Locate = %q, expected to contain granule ID", got)
	}
	if !strings.Contains(got, "cmr.earthdata.nasa.gov") {
		t.Errorf("Locate = %q, expected cmr.earthdata.nasa.gov", got)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("experiment", "some-id")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestHostWiring(t *testing.T) {
	info := Domain{}.Info()
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts[0] = %q, want %s", info.Hosts[0], Host)
	}
	if Host != "cmr.earthdata.nasa.gov" {
		t.Errorf("Host = %q, want cmr.earthdata.nasa.gov", Host)
	}
}
