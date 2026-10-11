package geo

import (
	"net"
	"testing"
)

func TestCityDatabase(t *testing.T) {
	if e := Load("testdata/GeoIP2-City-Test.mmdb"); e != nil {
		t.Fatal(e)
	}
	defer Close()
	l, ok := Lookup(net.ParseIP("81.2.69.142"))
	if !ok || l.Country != "GB" || l.City == "" || l.Region == "" {
		t.Fatalf("city lookup: %+v %v", l, ok)
	}
	if _, ok := Lookup(net.ParseIP("127.0.0.1")); ok {
		t.Fatal("invented localhost geography")
	}
}
