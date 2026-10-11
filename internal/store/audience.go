package store

import "strings"

// Ordered checks distinguish browsers that also advertise Chrome/Safari.
func ParseUserAgent(ua string) (browser, os string) {
	browser, os = "unknown", "unknown"
	for _, v := range []struct{ marker, name string }{{"Edg/", "edge"}, {"EdgA/", "edge"}, {"EdgiOS/", "edge"}, {"OPR/", "opera"}, {"YaBrowser/", "yandex"}, {"SamsungBrowser/", "samsung"}, {"Firefox/", "firefox"}, {"FxiOS/", "firefox"}, {"CriOS/", "chrome"}, {"Chrome/", "chrome"}, {"Version/", "safari"}} {
		if strings.Contains(ua, v.marker) {
			browser = v.name
			break
		}
	}
	for _, v := range []struct{ marker, name string }{{"Android", "android"}, {"iPhone", "ios"}, {"iPad", "ios"}, {"Windows NT", "windows"}, {"CrOS", "chromeos"}, {"Macintosh", "macos"}, {"Linux", "linux"}} {
		if strings.Contains(ua, v.marker) {
			os = v.name
			break
		}
	}
	return
}
