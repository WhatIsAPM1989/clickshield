package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSessionFilters(t *testing.T) {
	for _, tc := range []struct {
		query  string
		class  string
		paid   bool
		bot    string
		status int
	}{
		{"?classification=UNCLASSIFIED&paid_only=true", "UNCLASSIFIED", true, "", 200},
		{"?classification=bot", "BOT", false, "", 200},
		{"?bot=Googlebot&bot_category=search", "", false, "Googlebot", 200},
		{"?classification=unknown", "", false, "", 400},
		{"?bot_category=robots", "", false, "", 400},
		{"?paid_only=yes", "", false, "", 400},
	} {
		r := httptest.NewRequest("GET", "/v1/sessions"+tc.query, nil)
		w := httptest.NewRecorder()
		f, ok := sessionFilters(w, r)
		if ok != (tc.status == 200) || f.Class != tc.class || f.PaidOnly != tc.paid || f.BotName != tc.bot {
			t.Errorf("%s: filter=%+v ok=%v", tc.query, f, ok)
		}
		if !ok && w.Code != tc.status {
			t.Errorf("%s: status=%d", tc.query, w.Code)
		}
	}
}

func TestSafeCSVCell(t *testing.T) {
	for input, want := range map[string]string{"=1+1": "'=1+1", "+cmd": "'+cmd", "-2": "'-2", "@x": "'@x", "\t=1": "'\t=1", "normal": "normal", "": ""} {
		if got := safeCSVCell(input); got != want {
			t.Errorf("%q: got %q, want %q", input, got, want)
		}
	}
}
