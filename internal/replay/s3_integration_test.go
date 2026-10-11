package replay

import (
	"context"
	"github.com/lamooof/clickshield/internal/ident"
	"os"
	"testing"
)

func TestS3RoundtripAndRemoval(t *testing.T) {
	endpoint := os.Getenv("CLICKSHIELD_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("dedicated S3 endpoint required")
	}
	s, err := NewS3(S3Config{Endpoint: endpoint, Bucket: "clickshield-test", AccessKey: os.Getenv("CLICKSHIELD_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("CLICKSHIELD_TEST_S3_SECRET_KEY"), Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = s.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	id, _ := ident.UUID()
	key := "roundtrip/" + id
	defer s.Delete(ctx, key)
	if err = s.Put(ctx, key, []byte("recording")); err != nil {
		t.Fatal(err)
	}
	raw, err := s.Get(ctx, key)
	if err != nil || string(raw) != "recording" {
		t.Fatalf("S3 roundtrip: %q %v", raw, err)
	}
	if err = s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, key); err == nil {
		t.Fatal("removed object still accessible")
	}
}
