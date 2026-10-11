// Package bots identifies declared crawlers, link previewers, site auditors and
// automation clients by their user agent and by the IP ranges that crawler
// operators publish (ranges.go, refreshed with "go generate").
//
// The catalog is checked in order, so specific entries must precede broader
// ones (for example "Googlebot-Image" before "Googlebot"). After a catalog
// change, add a migration that sets sessions.bot_name to NULL: the server
// re-labels those sessions on start.
package bots

//go:generate go run ./genranges

import (
	"net/netip"
	"regexp"
	"strings"
)

// Categories. Every category except Automation marks a declared crawler that
// is classified as KNOWN_BOT; automation clients go through the regular
// scoring rules and end up as BOT.
const (
	Search     = "search"
	AI         = "ai"
	Social     = "social"
	SEO        = "seo"
	Ads        = "ads"
	Monitoring = "monitoring"
	Feeds      = "feeds"
	Security   = "security"
	Other      = "other"
	Automation = "automation"
)

type Bot struct {
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	Category string `json:"category"`
}

// Declared reports whether the bot announces itself as a crawler and should
// be classified as KNOWN_BOT rather than scored as a visitor.
func (b Bot) Declared() bool { return b.Name != "" && b.Category != Automation }

type entry struct {
	Bot
	markers []string
	require string
}

func e(name, vendor, category string, markers ...string) entry {
	return entry{Bot: Bot{name, vendor, category}, markers: markers}
}

// r restricts a Yandex entry to user agents that link to the robots page.
func r(x entry) entry {
	x.require = "/bots"
	return x
}

var catalog = []entry{
	// Google
	e("Googlebot-Image", "Google", Search, "googlebot-image"),
	e("Googlebot-Video", "Google", Search, "googlebot-video"),
	e("Googlebot-News", "Google", Search, "googlebot-news"),
	e("Googlebot", "Google", Search, "googlebot"),
	e("GoogleOther", "Google", Search, "googleother"),
	e("Google-InspectionTool", "Google", Monitoring, "google-inspectiontool"),
	e("Google StoreBot", "Google", Search, "storebot-google"),
	e("Google AdsBot", "Google", Ads, "adsbot-google"),
	e("Google AdSense", "Google", Ads, "mediapartners-google"),
	e("Google Feedfetcher", "Google", Feeds, "feedfetcher-google"),
	e("Google APIs", "Google", Other, "apis-google"),
	e("Google Favicon", "Google", Other, "google favicon"),
	e("Google Web Preview", "Google", Social, "google web preview", "google-pagerenderer"),
	e("Google Site Verification", "Google", Monitoring, "google-site-verification"),
	e("Google Read Aloud", "Google", Other, "google-read-aloud"),
	e("Google Safety", "Google", Security, "google-safety"),
	e("Google NotebookLM", "Google", AI, "google-notebooklm"),
	e("Google CloudVertexBot", "Google", AI, "google-cloudvertexbot"),
	e("Google Gemini", "Google", AI, "gemini-deep-research", "google-extended"),
	e("Google DuplexWeb", "Google", AI, "duplexweb-google"),
	e("Lighthouse / PageSpeed", "Google", Monitoring, "chrome-lighthouse", "google page speed", "pagespeed insights"),

	// Microsoft
	e("Bingbot", "Microsoft", Search, "bingbot", "msnbot"),
	e("BingPreview", "Microsoft", Social, "bingpreview", "microsoftpreview"),
	e("Bing AdIdxBot", "Microsoft", Ads, "adidxbot"),
	e("Skype / Teams preview", "Microsoft", Social, "skypeuripreview"),
	e("Google (спецкраулеры)", "Google", Other),
	e("Google (по запросу пользователя)", "Google", Other),

	// Yandex. Its apps ("YandexSearch/…") are browsers, so only robot names are
	// matched, and names shared with apps also need the yandex.com/bots link.
	e("YandexBot", "Yandex", Search, "yandexbot/", "yandexmobilebot"),
	e("YandexRenderResourcesBot", "Yandex", Search, "yandexrenderresourcesbot"),
	r(e("YandexImages", "Yandex", Search, "yandeximages", "yandeximageresizer")),
	r(e("YandexVideo", "Yandex", Search, "yandexvideo", "yandexmedia")),
	r(e("YandexNews", "Yandex", Feeds, "yandexnews", "yandexblogs")),
	r(e("YandexMarket", "Yandex", Search, "yandexmarket", "yandexsearchshop", "yandexvertis", "yandexverticals")),
	e("YandexDirect", "Yandex", Ads, "yandexdirect", "yandexadnet", "yandexpartner"),
	e("YandexMetrika", "Yandex", Monitoring, "yandexmetrika"),
	e("YandexWebmaster", "Yandex", Monitoring, "yandexwebmaster", "yandexpagechecker", "yandexaccessibilitybot", "yandexsitelinks"),
	e("YandexScreenshotBot", "Yandex", Other, "yandexscreenshotbot"),
	e("YandexFavicons", "Yandex", Other, "yandexfavicons"),
	e("YandexTurbo", "Yandex", Search, "yandexturbo"),
	e("YandexSpravBot", "Yandex", Search, "yandexspravbot"),
	e("YandexAdditional", "Yandex", AI, "yandexadditional"),
	e("YandexUserproxy", "Yandex", Other, "yandexuserproxy"),
	r(e("YandexCalendar", "Yandex", Other, "yandexcalendar")),
	e("Yandex (другие роботы)", "Yandex", Search, "yandex.com/bots", "yandex.ru/bots"),

	// Other search engines
	e("Mail.ru Bot", "VK", Search, "mail.ru_bot"),
	e("SputnikBot", "Rostelecom", Search, "sputnikbot"),
	e("StackRambler", "Rambler", Search, "stackrambler"),
	e("DuckDuckBot", "DuckDuckGo", Search, "duckduckbot", "duckduckgo-favicons-bot"),
	e("DuckAssistBot", "DuckDuckGo", AI, "duckassistbot"),
	e("Baiduspider", "Baidu", Search, "baiduspider"),
	e("Yahoo Slurp", "Yahoo", Search, "yahoo! slurp", "slurp"),
	e("Yahoo Japan", "Yahoo", Search, "y!j-"),
	e("Applebot", "Apple", Search, "applebot"),
	e("PetalBot", "Huawei", Search, "petalbot", "aspiegelbot"),
	e("SeznamBot", "Seznam", Search, "seznambot"),
	e("Naver Yeti", "Naver", Search, "yeti/", "naverbot"),
	e("Daum", "Kakao", Search, "daumoa"),
	e("Sogou", "Sogou", Search, "sogou web spider", "sogou inst spider"),
	e("360Spider", "Qihoo 360", Search, "360spider", "haosouspider"),
	e("YisouSpider", "Alibaba", Search, "yisouspider"),
	e("Qwant", "Qwant", Search, "qwantify", "qwantbot"),
	e("Coc Coc", "Coc Coc", Search, "coccocbot"),
	e("MojeekBot", "Mojeek", Search, "mojeekbot"),
	e("Exabot", "Exalead", Search, "exabot"),
	e("YepBot", "Ahrefs", Search, "yepbot"),
	e("Brave Search", "Brave", Search, "bravebot"),
	e("Meta WebIndexer", "Meta", Search, "meta-webindexer"),

	// AI crawlers and assistants
	e("GPTBot", "OpenAI", AI, "gptbot"),
	e("ChatGPT-User", "OpenAI", AI, "chatgpt-user"),
	e("OAI-SearchBot", "OpenAI", AI, "oai-searchbot"),
	e("ClaudeBot", "Anthropic", AI, "claudebot", "claude-searchbot", "claude-user", "claude-web", "anthropic-ai"),
	e("PerplexityBot", "Perplexity", AI, "perplexitybot", "perplexity-user"),
	e("CCBot", "Common Crawl", AI, "ccbot"),
	e("Bytespider", "ByteDance", AI, "bytespider"),
	e("Meta ExternalAgent", "Meta", AI, "meta-externalagent", "meta-externalfetcher", "facebookbot"),
	e("Amazonbot", "Amazon", AI, "amazonbot", "amzn-searchbot", "amzn-user"),
	e("Cohere", "Cohere", AI, "cohere-ai", "cohere-training-data-crawler"),
	e("MistralAI-User", "Mistral", AI, "mistralai-user"),
	e("DeepSeekBot", "DeepSeek", AI, "deepseekbot"),
	e("YouBot", "You.com", AI, "youbot"),
	e("PhindBot", "Phind", AI, "phindbot"),
	e("Diffbot", "Diffbot", AI, "diffbot"),
	e("AI2Bot", "Allen Institute", AI, "ai2bot"),
	e("Reflectionbot", "Reflection AI", AI, "reflectionbot"),
	e("Timpibot", "Timpi", AI, "timpibot"),
	e("ImagesiftBot", "ImageSift", AI, "imagesiftbot"),
	e("Omgili", "Webz.io", AI, "omgili", "webzio-extended"),
	e("Firecrawl", "Firecrawl", AI, "firecrawl"),
	e("Kangaroo Bot", "Kangaroo", AI, "kangaroo bot"),
	e("img2dataset", "LAION", AI, "img2dataset"),
	e("iAskSpider", "iAsk", AI, "iaskspider"),
	e("Amazon Bedrock / Nova Act", "Amazon", AI, "bedrockbot", "novaact"),
	e("ChatGLM Spider", "Zhipu AI", AI, "chatglm-spider"),
	e("Semantic Scholar", "Allen Institute", AI, "semanticscholarbot"),
	e("Velen", "Hunter.io", AI, "velenpublicwebcrawler"),
	e("QuillBot", "QuillBot", AI, "quillbot"),
	e("LinerBot", "Liner", AI, "linerbot"),
	e("TavilyBot", "Tavily", AI, "tavilybot"),

	// Social networks, messengers and link previews
	e("Facebook", "Meta", Social, "facebookexternalhit", "facebookcatalog"),
	// Telegram announces itself as "TelegramBot (like TwitterBot)".
	e("TelegramBot", "Telegram", Social, "telegrambot"),
	e("Twitterbot", "X", Social, "twitterbot"),
	e("LinkedInBot", "LinkedIn", Social, "linkedinbot"),
	e("WhatsApp", "Meta", Social, "whatsapp"),
	e("VK Share", "VK", Social, "vkshare", "vk.com/dev/share"),
	e("OdklBot", "VK", Social, "odklbot"),
	e("Slackbot", "Slack", Social, "slackbot", "slack-imgproxy"),
	e("Discordbot", "Discord", Social, "discordbot"),
	e("Pinterestbot", "Pinterest", Social, "pinterestbot", "pinterest/0."),
	e("Redditbot", "Reddit", Social, "redditbot"),
	e("Mastodon", "Mastodon", Social, "mastodon/"),
	e("Embedly", "Embedly", Social, "embedly"),
	e("Iframely", "Iframely", Social, "iframely"),
	e("Snapchat preview", "Snap", Social, "snap url preview"),
	e("KakaoTalk preview", "Kakao", Social, "kakaotalk-scrap"),
	e("Bitlybot", "Bitly", Social, "bitlybot"),
	e("Quora preview", "Quora", Social, "quora link preview"),
	e("Apple News", "Apple", Feeds, "applenewsbot"),

	// SEO and marketing intelligence
	e("SemrushBot", "Semrush", SEO, "semrushbot", "siteauditbot", "splitsignalbot"),
	e("AhrefsBot", "Ahrefs", SEO, "ahrefsbot", "ahrefssiteaudit"),
	e("MJ12bot", "Majestic", SEO, "mj12bot"),
	e("Moz", "Moz", SEO, "dotbot", "rogerbot"),
	e("DataForSeoBot", "DataForSEO", SEO, "dataforseobot"),
	e("SerpstatBot", "Serpstat", SEO, "serpstatbot"),
	e("BLEXBot", "WebMeUp", SEO, "blexbot"),
	e("Barkrowler", "Babbar", SEO, "barkrowler"),
	e("MegaIndex", "MegaIndex", SEO, "megaindex"),
	e("LinkpadBot", "Linkpad", SEO, "linkpadbot"),
	e("PR-CY", "PR-CY", SEO, "pr-cy"),
	e("SE Ranking", "SE Ranking", SEO, "seranking"),
	e("Keys.so", "Keys.so", SEO, "keys-so-bot", "keys.so"),
	e("Screaming Frog", "Screaming Frog", SEO, "screaming frog"),
	e("Sitebulb", "Sitebulb", SEO, "sitebulb"),
	e("Netpeak Spider", "Netpeak", SEO, "netpeakspider", "netpeak"),
	e("Seobility", "Seobility", SEO, "seobilitybot"),
	e("SISTRIX", "SISTRIX", SEO, "sistrix"),
	e("SEOkicks", "SEOkicks", SEO, "seokicks"),
	e("ZoominfoBot", "ZoomInfo", SEO, "zoominfobot"),
	e("Similarweb", "Similarweb", SEO, "similarweb", "similartech"),
	e("OpenLinkProfiler", "Axandra", SEO, "spbot/"),
	e("Linkdex", "Linkdex", SEO, "linkdexbot"),
	e("DomainStatsBot", "DomainStats", SEO, "domainstatsbot"),
	e("DomCop", "DomCop", SEO, "domcopbot"),
	e("Awario", "Awario", SEO, "awariobot", "awariosmartbot"),
	e("Mediatoolkit", "Mediatoolkit", SEO, "mediatoolkitbot"),
	e("Brandwatch", "Brandwatch", SEO, "magpie-crawler"),
	e("TurnitinBot", "Turnitin", SEO, "turnitinbot"),

	// Advertising and brand safety
	e("CriteoBot", "Criteo", Ads, "criteobot"),
	e("Amazon AdBot", "Amazon", Ads, "amazonadbot"),
	e("TaboolaBot", "Taboola", Ads, "taboolabot"),
	e("Outbrain", "Outbrain", Ads, "outbrain"),
	e("Grapeshot", "Oracle", Ads, "grapeshotcrawler"),
	e("Proximic", "Comscore", Ads, "proximic"),
	e("Peer39", "Peer39", Ads, "peer39_crawler"),
	e("ADmantX", "ADmantX", Ads, "admantx"),
	e("IAS crawler", "Integral Ad Science", Ads, "ias_crawler", "ias-crawler"),
	e("BidSwitchBot", "BidSwitch", Ads, "bidswitchbot"),

	// Monitoring, performance and validation
	e("UptimeRobot", "UptimeRobot", Monitoring, "uptimerobot"),
	e("Pingdom", "SolarWinds", Monitoring, "pingdom"),
	e("StatusCake", "StatusCake", Monitoring, "statuscake"),
	e("Site24x7", "Zoho", Monitoring, "site24x7"),
	e("New Relic", "New Relic", Monitoring, "newrelicpinger", "newrelic synthetics"),
	e("Datadog", "Datadog", Monitoring, "datadogsynthetics", "datadog agent"),
	e("Better Stack", "Better Stack", Monitoring, "betteruptime", "better uptime", "betterstack"),
	e("HetrixTools", "HetrixTools", Monitoring, "hetrixtools"),
	e("Freshping", "Freshworks", Monitoring, "freshping"),
	e("Uptime Kuma", "Uptime Kuma", Monitoring, "uptime-kuma"),
	e("updown.io", "updown.io", Monitoring, "updown.io"),
	e("GTmetrix", "GTmetrix", Monitoring, "gtmetrix"),
	e("WebPageTest", "Catchpoint", Monitoring, "ptst/", "webpagetest"),
	e("Catchpoint", "Catchpoint", Monitoring, "catchpoint"),
	e("Dynatrace", "Dynatrace", Monitoring, "ruxitsynthetic"),
	e("W3C Validator", "W3C", Monitoring, "w3c_validator", "w3c-checklink", "validator.nu"),

	// Archives and feed readers
	e("Internet Archive", "Internet Archive", Feeds, "archive.org_bot", "ia_archiver", "archive-it"),
	e("Feedly", "Feedly", Feeds, "feedly"),
	e("Inoreader", "Inoreader", Feeds, "inoreader"),
	e("NewsBlur", "NewsBlur", Feeds, "newsblur"),
	e("Feedbin", "Feedbin", Feeds, "feedbin"),

	// Internet-wide scanners
	e("Censys", "Censys", Security, "censysinspect", "censys"),
	e("Palo Alto Expanse", "Palo Alto Networks", Security, "expanse, a palo alto"),
	e("Netcraft", "Netcraft", Security, "netcraft"),
	e("InternetMeasurement", "Driftnet", Security, "internet-measurement"),
	e("Nuclei", "ProjectDiscovery", Security, "nuclei"),
	e("zgrab", "ZMap", Security, "zgrab"),
	e("Nmap", "Nmap", Security, "nmap scripting engine"),
	e("WPScan", "WPScan", Security, "wpscan"),

	// Browser automation and HTTP libraries. Not declared crawlers.
	e(Spoofed, "", Automation),
	e("HeadlessChrome", "", Automation, "headlesschrome"),
	e("PhantomJS", "", Automation, "phantomjs"),
	e("Selenium", "", Automation, "selenium"),
	e("Playwright", "", Automation, "playwright"),
	e("Puppeteer", "", Automation, "puppeteer"),
	e("Python requests", "", Automation, "python-requests"),
	e("Python urllib", "", Automation, "python-urllib"),
	e("Python httpx", "", Automation, "python-httpx"),
	e("aiohttp", "", Automation, "aiohttp"),
	e("Python (другое)", "", Automation, "python-", "python/"),
	e("Scrapy", "", Automation, "scrapy"),
	e("curl", "", Automation, "curl/"),
	e("Wget", "", Automation, "wget/"),
	e("Go HTTP client", "", Automation, "go-http-client"),
	e("OkHttp", "", Automation, "okhttp"),
	e("Apache HttpClient", "", Automation, "apache-httpclient"),
	e("Java HTTP client", "", Automation, "java/", "java-http-client"),
	e("node-fetch", "", Automation, "node-fetch"),
	e("axios", "", Automation, "axios/"),
	e("undici", "", Automation, "undici"),
	e("libwww-perl", "", Automation, "libwww-perl"),
	e("Guzzle", "", Automation, "guzzlehttp"),
	e("PHP", "", Automation, "php/"),
	e("Postman", "", Automation, "postmanruntime"),
	e("HTTPie", "", Automation, "httpie/"),
	e("PowerShell", "", Automation, "windowspowershell"),
}

// Generic crawler conventions. They do not match device names such as
// "CUBOT P40", which contain "bot" but no version slash.
var genericMarkers = []string{"bot/", "crawler", "spider", "+http://", "+https://", "compatible; robot", "; robot;"}

var (
	botToken        = regexp.MustCompile(`(?i)[a-z0-9][a-z0-9._-]*(?:bot|crawler|spider)[a-z0-9._-]*`)
	urlToken        = regexp.MustCompile(`(?i)\+?https?://\S*|\S+@\S+`)
	compatibleToken = regexp.MustCompile(`(?i)compatible;\s*([^;/()]+)`)
	productToken    = regexp.MustCompile(`^([^\s/;()]+)`)
)

// Spoofed names a client that claims to be a verifiable crawler from an
// address outside that operator's published ranges.
const Spoofed = "Поддельный краулер"

// verified lists crawlers whose operators publish every address they crawl
// from (ranges.go). Their user agent from any other address is a forgery, so
// it must not earn the KNOWN_BOT exemption. Lighthouse, link previewers and
// bots without published ranges are not verified this way.
var verified = map[string]bool{
	"Googlebot": true, "Googlebot-Image": true, "Googlebot-Video": true, "Googlebot-News": true,
	"GoogleOther": true, "Google StoreBot": true, "Google AdsBot": true, "Google AdSense": true,
	"Bingbot": true, "Applebot": true, "DuckDuckBot": true,
	"GPTBot": true, "ChatGPT-User": true, "OAI-SearchBot": true, "PerplexityBot": true,
}

var byName = func() map[string]Bot {
	m := make(map[string]Bot, len(catalog))
	for _, x := range catalog {
		m[x.Name] = x.Bot
	}
	return m
}()

// byPrefix indexes publishedRanges by prefix length.
var byPrefix = func() map[int]map[netip.Prefix]string {
	m := map[int]map[netip.Prefix]string{}
	for _, r := range publishedRanges {
		p := netip.MustParsePrefix(r.prefix)
		if _, ok := byName[r.bot]; !ok {
			panic("bots: range for unknown bot " + r.bot)
		}
		if m[p.Bits()] == nil {
			m[p.Bits()] = map[netip.Prefix]string{}
		}
		m[p.Bits()][p] = r.bot
	}
	return m
}()

// Identify returns the bot behind a request, or false for browsers. A request
// from a published crawler range is that crawler unless its user agent names
// another declared bot: crawlers such as Bing render pages with a plain
// HeadlessChrome user agent. A verifiable crawler user agent from an address
// outside its operator's ranges is reported as Spoofed; without an address it
// cannot be checked and keeps its name.
func Identify(ua, ip string) (Bot, bool) {
	bot, ok := identifyUA(ua)
	if ok && bot.Declared() {
		if verified[bot.Name] {
			if _, err := netip.ParseAddr(ip); err == nil {
				if b, found := identifyIP(ip); !found || b.Vendor != bot.Vendor {
					return byName[Spoofed], true
				}
			}
		}
		return bot, true
	}
	if b, found := identifyIP(ip); found {
		return b, true
	}
	return bot, ok
}

func identifyIP(ip string) (Bot, bool) {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return Bot{}, false
	}
	a = a.Unmap()
	for bits, prefixes := range byPrefix {
		p, err := a.Prefix(bits)
		if err != nil {
			continue
		}
		if name, ok := prefixes[p]; ok {
			return byName[name], true
		}
	}
	return Bot{}, false
}

func identifyUA(ua string) (Bot, bool) {
	u := strings.ToLower(ua)
	if u == "" {
		return Bot{}, false
	}
	for _, x := range catalog {
		if x.require != "" && !strings.Contains(u, x.require) {
			continue
		}
		for _, m := range x.markers {
			if strings.Contains(u, m) {
				return x.Bot, true
			}
		}
	}
	for _, m := range genericMarkers {
		if strings.Contains(u, m) {
			return Bot{Name: genericName(ua), Category: Other}, true
		}
	}
	return Bot{}, false
}

// Lookup returns catalog metadata for a stored bot name. Unknown generic
// crawlers come back with an empty vendor and the Other category.
func Lookup(name string) Bot {
	if b, ok := byName[name]; ok {
		return b
	}
	return Bot{Name: name, Category: Other}
}

// Catalog lists every known bot for display.
func Catalog() []Bot {
	out := make([]Bot, len(catalog))
	for i, x := range catalog {
		out[i] = x.Bot
	}
	return out
}

// genericName extracts a readable crawler name from an undeclared user agent,
// e.g. "SomeCrawler" from "Mozilla/5.0 (compatible; SomeCrawler/2.0; +https://…)".
func genericName(ua string) string {
	ua = urlToken.ReplaceAllString(ua, "")
	name := botToken.FindString(ua)
	if name == "" {
		if m := compatibleToken.FindStringSubmatch(ua); m != nil {
			name = strings.TrimSpace(m[1])
		}
	}
	if name == "" || strings.EqualFold(name, "mozilla") {
		if m := productToken.FindStringSubmatch(ua); m != nil {
			name = m[1]
		}
	}
	if name == "" {
		name = "Неизвестный краулер"
	}
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}
