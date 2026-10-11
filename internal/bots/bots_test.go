package bots

import "testing"

func TestIdentify(t *testing.T) {
	for ua, want := range map[string]Bot{
		"Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.8010.52 Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)": {"Googlebot", "Google", Search},
		"Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.8010.52 Mobile Safari/537.36 (compatible; GoogleOther)":                                    {"GoogleOther", "Google", Search},
		"Googlebot-Image/1.0":                                {"Googlebot-Image", "Google", Search},
		"AdsBot-Google (+http://www.google.com/adsbot.html)": {"Google AdsBot", "Google", Ads},
		"Google-NotebookLM":                                  {"Google NotebookLM", "Google", AI},
		"meta-externalagent/1.1 (+https://developers.facebook.com/docs/sharing/webmasters/crawler)": {"Meta ExternalAgent", "Meta", AI},
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36 Edg/145.0.0.0 (compatible; meta-webindexer/1.1 (+https://developers.facebook.com/docs/sharing/webmasters/crawler))": {"Meta WebIndexer", "Meta", Search},
		"Mozilla/5.0 (compatible; Baiduspider-render/2.0; +http://www.baidu.com/search/spider.html)":                                                                                                                                         {"Baiduspider", "Baidu", Search},
		"Mozilla/5.0 (compatible; YandexRenderResourcesBot/1.0; +http://yandex.com/bots) AppleWebKit/537.36 (KHTML, like Gecko)":                                                                                                             {"YandexRenderResourcesBot", "Yandex", Search},
		"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)":                                                                                                                                                                   {"YandexBot", "Yandex", Search},
		"Mozilla/5.0 (compatible; YandexMarket/1.0; +http://yandex.com/bots)":                                                                                                                                                                {"YandexMarket", "Yandex", Search},
		"Mozilla/5.0 (compatible; YandexUserproxy; robot; +http://yandex.com/bots)":                                                                                                                                                          {"YandexUserproxy", "Yandex", Other},
		"Mozilla/5.0 (compatible; YandexSomethingNew/1.0; +http://yandex.com/bots)":                                                                                                                                                          {"Yandex (другие роботы)", "Yandex", Search},
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36; compatible; OAI-SearchBot/1.4; +https://openai.com/searchbot":                                                {"OAI-SearchBot", "OpenAI", AI},
		"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)":                                                                                                                                                                 {"AhrefsBot", "Ahrefs", SEO},
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15 (Applebot/0.1; +http://www.apple.com/go/applebot)":                                                            {"Applebot", "Apple", Search},
		"Mozilla/5.0 (Linux; Android 5.0) AppleWebKit/537.36 (KHTML, like Gecko) Mobile Safari/537.36 (compatible; Bytespider; spider-feedback@bytedance.com)":                                                                               {"Bytespider", "ByteDance", AI},
		"Mozilla/5.0 (Linux; Android 11; moto g power (2022)) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Mobile Safari/537.36 Chrome-Lighthouse":                                                                                {"Lighthouse / PageSpeed", "Google", Monitoring},
		"Mozilla/5.0 (compatible; PR-CY)": {"PR-CY", "PR-CY", SEO},
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; Reflectionbot/1.0; +https://reflection.ai/bot) Chrome/151.0.0.0 Safari/537.36": {"Reflectionbot", "Reflection AI", AI},
		"TelegramBot (like TwitterBot)": {"TelegramBot", "Telegram", Social},
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/146.0.7680.165 Safari/537.36": {"HeadlessChrome", "", Automation},
		"Python-urllib/3.14": {"Python urllib", "", Automation},
		"curl/8.5.0":         {"curl", "", Automation},
		// Undeclared crawlers get a name taken from the user agent.
		"Mozilla/5.0 (compatible; FooCrawler/2.0; +https://foo.example/bot.html)": {"FooCrawler", "", Other},
		"Mozilla/5.0 (compatible; Acme/1.0; +https://acme.example/robots)":        {"Acme", "", Other},
		"ExampleBot/1.0": {"ExampleBot", "", Other},
	} {
		got, ok := Identify(ua, "")
		if !ok || got != want {
			t.Errorf("%s:\n got %+v (%v), want %+v", ua, got, ok, want)
		}
	}
}

func TestBrowsersAreNotBots(t *testing.T) {
	for _, ua := range []string{
		"",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 YaBrowser/26.8.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.6 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Linux; Android 10; CUBOT P40) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 14; TECNO LI6 Build/UP1A.231005.007; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/117.0.0.0 Mobile Safari/537.36 YandexSearch/24.1",
		"Mozilla/5.0 (Phone; OpenHarmony 7.0; Android 10) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36  ArkWeb/7.0.0.105 Mobile HuaweiBrowser/6.1.7.303",
		"Mozilla/5.0 (Linux; Android 13; SM-A536B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36 YandexMarket/5.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.33 Safari/537.36 Edg/131.0.6778.33",
	} {
		if b, ok := Identify(ua, "203.0.113.7"); ok {
			t.Errorf("browser identified as %+v: %s", b, ua)
		}
	}
}

func TestIdentifyByPublishedRanges(t *testing.T) {
	const headless = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/146.0.7680.165 Safari/537.36"
	const chrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	for _, c := range []struct {
		ua, ip, want string
	}{
		{headless, "40.77.177.31", "Bingbot"},
		{headless, "40.77.188.66", "Bingbot"},
		{headless, "199.30.24.10", "Bingbot"},
		{headless, "65.55.210.4", "Bingbot"},
		{chrome, "::ffff:40.77.179.24", "Bingbot"},
		{chrome, "66.249.66.76", "Googlebot"},
		{"", "66.249.66.76", "Googlebot"},
		// A declared bot keeps its own name.
		{"Mozilla/5.0 (compatible; GoogleOther)", "66.249.66.76", "GoogleOther"},
		// Outside published ranges the user agent decides.
		{headless, "40.77.1.1", "HeadlessChrome"},
	} {
		got, ok := Identify(c.ua, c.ip)
		if !ok || got.Name != c.want {
			t.Errorf("%s %s: got %+v (%v), want %s", c.ip, c.ua, got, ok, c.want)
		}
	}
	if b, ok := Identify(headless, "40.77.177.31"); !b.Declared() {
		t.Errorf("Bing renderer not declared: %+v %v", b, ok)
	}
	if b, ok := Identify(chrome, "95.24.49.142"); ok {
		t.Errorf("browser identified as %+v", b)
	}
}

func TestCatalogNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range Catalog() {
		if seen[b.Name] {
			t.Errorf("duplicate bot name %q", b.Name)
		}
		seen[b.Name] = true
		if Lookup(b.Name) != b {
			t.Errorf("lookup mismatch for %q", b.Name)
		}
	}
}

func TestSpoofedCrawlers(t *testing.T) {
	const googlebot = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	for _, c := range []struct {
		ua, ip, want string
	}{
		{googlebot, "66.249.66.76", "Googlebot"},
		{googlebot, "", "Googlebot"},
		{googlebot, "95.24.49.142", Spoofed},
		// Bing's address does not vouch for a Google user agent.
		{googlebot, "40.77.177.31", Spoofed},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", "185.220.101.1", Spoofed},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", "40.77.177.31", "Bingbot"},
		// Crawlers without published ranges are not checked.
		{"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)", "95.24.49.142", "YandexBot"},
		{"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)", "95.24.49.142", "AhrefsBot"},
	} {
		got, ok := Identify(c.ua, c.ip)
		if !ok || got.Name != c.want {
			t.Errorf("%s %s: got %+v (%v), want %s", c.ip, c.ua, got, ok, c.want)
		}
	}
	if b, _ := Identify(googlebot, "95.24.49.142"); b.Declared() || b.Category != Automation {
		t.Errorf("spoofed crawler must be scored as automation: %+v", b)
	}
	for name := range verified {
		if _, ok := byName[name]; !ok {
			t.Errorf("verified crawler %q is not in the catalog", name)
		}
	}
}
