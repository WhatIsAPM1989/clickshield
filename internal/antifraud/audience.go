package antifraud

import "github.com/lamooof/clickshield/internal/clientmeta"

func ParseUserAgent(ua string) (string, string) { return clientmeta.ParseUserAgent(ua) }
