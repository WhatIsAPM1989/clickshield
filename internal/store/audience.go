package store

import "github.com/lamooof/clickshield/internal/antifraud"

func ParseUserAgent(ua string) (string, string) { return antifraud.ParseUserAgent(ua) }
