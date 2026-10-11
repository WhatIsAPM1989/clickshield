package store

import (
	"context"
	"errors"
	"github.com/lamooof/clickshield/internal/replay"
	"os"
	"testing"
)

type testObjects map[string][]byte

func (o testObjects) Put(_ context.Context, key string, raw []byte) error {
	o[key] = append([]byte(nil), raw...)
	return nil
}
func (o testObjects) Get(_ context.Context, key string) ([]byte, error) {
	raw, ok := o[key]
	if !ok {
		return nil, errors.New("object missing")
	}
	return append([]byte(nil), raw...), nil
}
func (o testObjects) Delete(_ context.Context, key string) error { delete(o, key); return nil }

func testReplayObjects(t *testing.T) replay.Objects {
	t.Helper()
	var objects replay.Objects = testObjects{}
	if endpoint := os.Getenv("CLICKSHIELD_TEST_S3_ENDPOINT"); endpoint != "" {
		s3, err := replay.NewS3(replay.S3Config{Endpoint: endpoint, Bucket: "clickshield-test", AccessKey: os.Getenv("CLICKSHIELD_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("CLICKSHIELD_TEST_S3_SECRET_KEY"), Region: "us-east-1"})
		if err != nil {
			t.Fatal(err)
		}
		if err = s3.EnsureBucket(context.Background()); err != nil {
			t.Fatal(err)
		}
		objects = s3
	}
	return objects
}

func testReplayService(t *testing.T, st *Store) *replay.Service {
	return replay.NewService(replay.NewRepository(st.Pool), testReplayObjects(t))
}
