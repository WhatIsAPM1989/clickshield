package traffic

import (
	"github.com/lamooof/clickshield/internal/domain"
	"net/url"
	"strings"
)

type Attribution struct {
	Source       string `json:"source"`
	Channel      string `json:"channel"`
	ReferrerHost string `json:"referrer_host"`
	Basis        string `json:"basis"`
}

func host(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
}
func matches(h, d string) bool { return h == d || strings.HasSuffix(h, "."+d) }
func source(h string) string {
	if matches(h, "gemini.google.com") {
		return "gemini"
	}
	for _, p := range []struct {
		name    string
		domains []string
	}{
		{"yandex", []string{"yandex.ru", "yandex.com", "ya.ru", "yandex.kz", "yandex.by"}},
		{"google", []string{"google.com", "google.ru", "google.co.uk", "google.de"}},
		{"bing", []string{"bing.com"}}, {"mail.ru", []string{"go.mail.ru"}},
		{"vk", []string{"vk.com", "vk.ru"}}, {"telegram", []string{"t.me", "telegram.org"}},
		{"youtube", []string{"youtube.com", "youtu.be"}}, {"facebook", []string{"facebook.com"}},
		{"instagram", []string{"instagram.com"}}, {"ok", []string{"ok.ru"}},
		{"chatgpt", []string{"chatgpt.com", "chat.openai.com"}}, {"perplexity", []string{"perplexity.ai"}},
		{"claude", []string{"claude.ai"}}, {"gemini", []string{"gemini.google.com"}},
	} {
		for _, d := range p.domains {
			if matches(h, d) {
				return p.name
			}
		}
	}
	return h
}
func paid(m string) bool {
	switch m {
	case "cpc", "ppc", "paid", "paidsearch", "paid_search", "paid-social", "paid_social", "display", "cpm", "retargeting":
		return true
	}
	return false
}
func Classify(b domain.EventBatch, origins []string) Attribution {
	h := host(b.Referrer)
	internal := false
	for _, o := range append(append([]string{}, origins...), b.LandingURL) {
		if h != "" && h == host(o) {
			internal = true
		}
	}
	a := Attribution{Source: "direct", Channel: "direct_unknown", Basis: "no_referrer"}
	if !internal {
		a.ReferrerHost = h
	}
	s, m := strings.ToLower(strings.TrimSpace(b.UTM.Source)), strings.ToLower(strings.TrimSpace(b.UTM.Medium))
	if s != "" || m != "" || b.UTM.Campaign != "" {
		a.Source = s
		if a.Source == "" {
			a.Source = "unknown"
		}
		a.Channel = "tagged"
		a.Basis = "utm"
	}
	switch {
	case b.YCLID != "":
		a.Source = "yandex"
		a.Channel = "paid"
		a.Basis = "yclid"
	case b.GCLID != "":
		a.Source = "google"
		a.Channel = "paid"
		a.Basis = "gclid"
	case b.RBClickID != "":
		a.Source = "vk"
		a.Channel = "paid_social"
		a.Basis = "rb_clickid"
	case paid(m):
		a.Channel = "paid"
		if m == "display" || m == "cpm" {
			a.Channel = "display"
		}
		if m == "paid_social" || m == "paid-social" {
			a.Channel = "paid_social"
		}
	case m == "email":
		a.Channel = "email"
	case m == "social" || m == "social-network":
		a.Channel = "social"
	case m == "organic":
		a.Channel = "organic_search"
	case a.Basis == "utm":
	case h != "" && !internal:
		a.Source = source(h)
		a.Basis = "referrer"
		a.Channel = "referral"
		switch a.Source {
		case "yandex", "google", "bing", "mail.ru":
			a.Channel = "organic_search"
		case "vk", "telegram", "youtube", "facebook", "instagram", "ok":
			a.Channel = "social"
		case "chatgpt", "perplexity", "claude", "gemini":
			a.Channel = "ai"
		}
	case internal:
		a.Basis = "internal_referrer"
	}
	return a
}
