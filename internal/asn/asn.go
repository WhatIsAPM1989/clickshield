// Package asn maps IP addresses to autonomous systems with the iptoasn.com
// database (ip2asn-combined.tsv.gz, public domain) and tells which systems
// belong to cloud and hosting providers. The database is loaded once at
// start; until then, or without it, lookups find nothing.
package asn

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

type Info struct {
	Number  uint32
	Org     string
	Country string
}

type span struct {
	start, end netip.Addr
	info       int32
}

type DB struct {
	spans []span
	infos []Info
}

var current atomic.Pointer[DB]

// Load reads a gzip-compressed ip2asn TSV file and makes it the database used
// by Lookup.
func Load(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	db, err := Parse(z)
	if err != nil {
		return 0, err
	}
	current.Store(db)
	return len(db.spans), nil
}

// Parse reads ip2asn TSV rows: range_start, range_end, AS_number,
// country_code, AS_description. Unrouted ranges (AS 0) are skipped.
func Parse(r io.Reader) (*DB, error) {
	db := &DB{}
	index := map[Info]int32{}
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 5 {
			return nil, fmt.Errorf("line %d: expected 5 fields", line)
		}
		n, err := strconv.ParseUint(f[2], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if n == 0 {
			continue
		}
		start, err1 := netip.ParseAddr(f[0])
		end, err2 := netip.ParseAddr(f[1])
		if err1 != nil || err2 != nil || start.Is4() != end.Is4() {
			return nil, fmt.Errorf("line %d: invalid range", line)
		}
		info := Info{Number: uint32(n), Org: f[4], Country: f[3]}
		if info.Country == "None" {
			info.Country = ""
		}
		i, ok := index[info]
		if !ok {
			i = int32(len(db.infos))
			index[info] = i
			db.infos = append(db.infos, info)
		}
		db.spans = append(db.spans, span{start, end, i})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Slice(db.spans, func(a, b int) bool { return db.spans[a].start.Less(db.spans[b].start) })
	return db, nil
}

// Lookup returns the autonomous system announcing ip.
func Lookup(ip string) (Info, bool) {
	db := current.Load()
	a, err := netip.ParseAddr(ip)
	if db == nil || err != nil {
		return Info{}, false
	}
	return db.Lookup(a.Unmap())
}

func (db *DB) Lookup(a netip.Addr) (Info, bool) {
	i := sort.Search(len(db.spans), func(i int) bool { return a.Less(db.spans[i].start) }) - 1
	if i < 0 || db.spans[i].start.Is4() != a.Is4() || db.spans[i].end.Less(a) {
		return Info{}, false
	}
	return db.infos[db.spans[i].info], true
}

// ParseNumber accepts "AS15169" or "15169" and returns 0 for anything else.
func ParseNumber(s string) uint32 {
	s = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(s)), "AS")
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}
