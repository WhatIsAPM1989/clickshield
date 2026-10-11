package config

import "testing"

func TestMaintenanceDoesNotRequireApplicationSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	for _, key := range []string{"SESSION_SECRET", "ADMIN_PASSWORD", "ENCRYPTION_KEY"} {
		t.Setenv(key, "")
	}
	for _, command := range []string{"migrate", "backfill"} {
		if _, err := LoadFor(command); err != nil {
			t.Errorf("%s: %v", command, err)
		}
	}
	if _, err := LoadFor("serve"); err == nil {
		t.Fatal("serve accepted missing secrets")
	}
}
