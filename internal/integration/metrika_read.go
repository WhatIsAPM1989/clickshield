package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type MetrikaReader struct {
	Client   *http.Client
	Endpoint string
}

func NewMetrikaReader() *MetrikaReader {
	return &MetrikaReader{Client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, Endpoint: "https://api-metrika.yandex.net/stat/v1/data"}
}

type MetrikaDimension struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type MetrikaRow struct {
	Dimensions []MetrikaDimension `json:"dimensions"`
	Metrics    []float64          `json:"metrics"`
}
type MetrikaReport struct {
	Data        []MetrikaRow `json:"data"`
	TotalRows   int          `json:"total_rows"`
	Sampled     bool         `json:"sampled"`
	SampleShare float64      `json:"sample_share"`
	Totals      []float64    `json:"totals"`
}

func (r *MetrikaReader) Report(ctx context.Context, token, counter, kind, from, to string, offset int) (MetrikaReport, error) {
	dims := map[string]string{"sources": "ym:s:lastTrafficSource,ym:s:lastSourceEngine,ym:s:lastDirectPlatformType", "campaigns": "ym:s:lastDirectClickOrder", "platforms": "ym:s:lastDirectPlatformType,ym:s:lastDirectPlatform", "utm": "ym:s:lastUTMSource,ym:s:lastUTMMedium,ym:s:lastUTMCampaign"}[kind]
	if dims == "" {
		return MetrikaReport{}, fmt.Errorf("unknown report")
	}
	q := url.Values{"ids": {counter}, "dimensions": {dims}, "metrics": {"ym:s:visits,ym:s:users,ym:s:bounceRate,ym:s:avgVisitDurationSeconds"}, "date1": {from}, "date2": {to}, "accuracy": {"full"}, "lang": {"ru"}, "limit": {"500"}, "offset": {strconv.Itoa(offset)}, "sort": {"-ym:s:visits"}}
	if kind == "campaigns" || kind == "platforms" {
		q.Set("filters", "ym:s:lastAdvEngine=='ya_direct'")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, r.Endpoint+"?"+q.Encode(), nil)
	if e != nil {
		return MetrikaReport{}, fmt.Errorf("could not build Metrika request")
	}
	req.Header.Set("Authorization", "OAuth "+token)
	resp, e := r.Client.Do(req)
	if e != nil {
		return MetrikaReport{}, fmt.Errorf("Metrika request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return MetrikaReport{}, fmt.Errorf("Metrika HTTP %d (check OAuth metrika:read access and counter)", resp.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if e != nil || len(raw) > 4<<20 {
		return MetrikaReport{}, fmt.Errorf("Metrika response too large")
	}
	var report MetrikaReport
	if e = json.Unmarshal(raw, &report); e != nil {
		return report, fmt.Errorf("invalid Metrika response")
	}
	if report.Data == nil {
		report.Data = []MetrikaRow{}
	}
	return report, nil
}
