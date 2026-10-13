package store

import (
	"context"
	"net"

	"github.com/lamooof/clickshield/internal/geo"
)

// FillGeo fills country, region and city from the loaded City MMDB for
// sessions stored without a location. Projections are queued by the
// sessions trigger.
func (s *Store) FillGeo(ctx context.Context) (filled int64, err error) {
	last := "00000000-0000-0000-0000-000000000000"
	for {
		rows, e := s.Pool.Query(ctx, `SELECT id::text,host(ip) FROM sessions WHERE country='' AND ip IS NOT NULL AND id>$1::uuid ORDER BY id LIMIT 1000`, last)
		if e != nil {
			return filled, e
		}
		var ids, countries, regions, cities []string
		n := 0
		for rows.Next() {
			var id, ip string
			if e = rows.Scan(&id, &ip); e != nil {
				rows.Close()
				return filled, e
			}
			n, last = n+1, id
			if l, ok := geo.Lookup(net.ParseIP(ip)); ok {
				ids, countries, regions, cities = append(ids, id), append(countries, l.Country), append(regions, l.Region), append(cities, l.City)
			}
		}
		rows.Close()
		if e = rows.Err(); e != nil || n == 0 {
			return filled, e
		}
		if len(ids) == 0 {
			continue
		}
		tag, e := s.Pool.Exec(ctx, `UPDATE sessions s SET country=left(v.country,8),region=left(v.region,120),city=left(v.city,120),data_revision=s.data_revision+1 FROM unnest($1::uuid[],$2::text[],$3::text[],$4::text[]) AS v(id,country,region,city) WHERE s.id=v.id`, ids, countries, regions, cities)
		if e != nil {
			return filled, e
		}
		filled += tag.RowsAffected()
	}
}
