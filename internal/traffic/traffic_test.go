package traffic

import (
	"github.com/lamooof/clickshield/internal/domain"
	"testing"
)

func TestAttribution(t *testing.T) {
	tests := []struct {
		name            string
		b               domain.EventBatch
		source, channel string
	}{
		{"no referrer", domain.EventBatch{}, "direct", "direct_unknown"},
		{"yandex search", domain.EventBatch{Referrer: "https://yandex.ru/search"}, "yandex", "organic_search"},
		{"external referral", domain.EventBatch{Referrer: "https://blog.example.net/post"}, "blog.example.net", "referral"},
		{"internal", domain.EventBatch{Referrer: "https://www.example.com/a"}, "direct", "direct_unknown"},
		{"spoofed search", domain.EventBatch{Referrer: "https://yandex.ru.evil.test"}, "yandex.ru.evil.test", "referral"},
		{"paid overrides referrer", domain.EventBatch{Referrer: "https://google.com", YCLID: "123"}, "yandex", "paid"},
		{"utm", domain.EventBatch{UTM: domain.UTM{Source: "telegram", Medium: "social"}}, "telegram", "social"},
		{"vk", domain.EventBatch{RBClickID: "x"}, "vk", "paid_social"},
		{"email", domain.EventBatch{UTM: domain.UTM{Source: "newsletter", Medium: "email"}}, "newsletter", "email"},
		{"AI", domain.EventBatch{Referrer: "https://chatgpt.com/"}, "chatgpt", "ai"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := Classify(tc.b, []string{"https://example.com", "https://www.example.com"})
			if a.Source != tc.source || a.Channel != tc.channel {
				t.Fatalf("attribution=%+v", a)
			}
		})
	}
}
