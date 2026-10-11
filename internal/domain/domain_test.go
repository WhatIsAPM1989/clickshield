package domain

import "testing"

func TestNormalizeMode(t *testing.T) {
	for in, want := range map[string]string{
		"":              ModeMonitoring,
		"MONITORING":    ModeMonitoring,
		"NOTIFY":        ModeNotify,
		"BLOCKING":      ModeBlocking,
		"BLOCKING_BOTS": ModeBlockingBots,
		"blocking":      ModeMonitoring,
		"DROP":          ModeMonitoring,
	} {
		if got := NormalizeMode(in); got != want {
			t.Errorf("NormalizeMode(%q) = %q, want %q", in, got, want)
		}
	}
}
