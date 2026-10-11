package asn

import (
	"net/netip"
	"strings"
	"testing"
)

const sample = `1.0.0.0	1.0.0.255	13335	US	CLOUDFLARENET
1.0.1.0	1.0.3.255	0	None	Not routed
34.34.0.0	34.34.255.255	396982	US	GOOGLE-CLOUD-PLATFORM
95.24.0.0	95.24.255.255	8402	RU	CORBINA-AS
2001:4860::	2001:4860:ffff:ffff:ffff:ffff:ffff:ffff	15169	US	GOOGLE
`

func TestLookup(t *testing.T) {
	db, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[string]uint32{
		"1.0.0.7": 13335, "34.34.247.145": 396982, "95.24.49.142": 8402, "2001:4860::8888": 15169,
		"1.0.2.1": 0, "34.35.0.1": 0, "0.0.0.1": 0, "2001:4861::1": 0, "::1": 0,
	} {
		got, ok := db.Lookup(netip.MustParseAddr(ip))
		if got.Number != want || ok != (want != 0) {
			t.Errorf("%s: got %+v %v, want AS%d", ip, got, ok, want)
		}
	}
}

func TestClassify(t *testing.T) {
	for s, want := range map[string]Network{"AS45102": Cloud, "396982": Cloud, "as210644": Hosting, "8402": Residential, "": Residential} {
		if got := Classify(ParseNumber(s)); got != want {
			t.Errorf("%q: got %v, want %v", s, got, want)
		}
	}
	seen := map[string]bool{}
	for _, n := range Datacenter() {
		if seen[n] {
			t.Errorf("AS%s is listed twice", n)
		}
		seen[n] = true
	}
}
