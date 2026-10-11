// Package geo reads an optional local GeoIP2/GeoLite2 City MMDB without external requests.
package geo

import (
	"net"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

var mu sync.RWMutex
var database *maxminddb.Reader

type Location struct{ Country, Region, City string }
type record struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Subdivisions []struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
}

func Load(path string) error {
	r, e := maxminddb.Open(path)
	if e != nil {
		return e
	}
	mu.Lock()
	defer mu.Unlock()
	if database != nil {
		database.Close()
	}
	database = r
	return nil
}
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if database != nil {
		database.Close()
		database = nil
	}
}
func name(names map[string]string) string {
	if v := names["ru"]; v != "" {
		return v
	}
	return names["en"]
}
func Lookup(ip net.IP) (Location, bool) {
	mu.RLock()
	defer mu.RUnlock()
	if database == nil || ip == nil {
		return Location{}, false
	}
	var r record
	if e := database.Lookup(ip, &r); e != nil {
		return Location{}, false
	}
	l := Location{Country: r.Country.ISOCode, City: name(r.City.Names)}
	if len(r.Subdivisions) > 0 {
		l.Region = name(r.Subdivisions[0].Names)
	}
	return l, l.Country != "" || l.City != "" || l.Region != ""
}
