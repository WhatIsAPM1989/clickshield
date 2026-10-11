package main

import "testing"

func TestCLICommands(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"serve"}, {"worker"}, {"migrate"}, {"bootstrap"}, {"backfill", "bots"}, {"rescore", "12345678-1234-1234-1234-123456789abc"}, {"reproduce", "42"}} {
		if _, _, err := parseCommand(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"unknown"}, {"serve", "extra"}, {"rescore"}, {"rescore", "invalid"}, {"backfill"}, {"backfill", "everything"}, {"reproduce"}} {
		if _, _, err := parseCommand(args); err == nil {
			t.Errorf("invalid command accepted: %v", args)
		}
	}
}
