package replay

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryMetadata struct {
	Metadata
	chunk Chunk
	ready bool
	marks int
}

func (m *memoryMetadata) Reserve(_ context.Context, _ string, c Chunk) (bool, error) {
	m.chunk = c
	return m.ready, nil
}
func (m *memoryMetadata) MarkReady(context.Context, Chunk) error {
	m.ready = true
	m.marks++
	return nil
}

type memoryObjects struct {
	data map[string][]byte
	fail bool
	puts int
}

func (o *memoryObjects) Put(_ context.Context, key string, raw []byte) error {
	o.puts++
	if o.fail {
		return errors.New("S3 unavailable")
	}
	o.data[key] = raw
	return nil
}
func (o *memoryObjects) Get(_ context.Context, key string) ([]byte, error) { return o.data[key], nil }
func (o *memoryObjects) Delete(_ context.Context, key string) error        { delete(o.data, key); return nil }

func TestUploadFailureRemainsRetryableAndReadyRetrySkipsPut(t *testing.T) {
	m := &memoryMetadata{}
	o := &memoryObjects{data: map[string][]byte{}, fail: true}
	s := NewService(m, o)
	p := Part{SessionID: "session", PageID: "page", Data: []byte(`[{"type":4,"timestamp":1}]`)}
	if err := s.SaveReplayPart(context.Background(), "project", p); err == nil || m.marks != 0 {
		t.Fatal("failed upload became ready")
	}
	key := m.chunk.Key
	o.fail = false
	if err := s.SaveReplayPart(context.Background(), "project", p); err != nil {
		t.Fatal(err)
	}
	if m.chunk.Key != key || m.marks != 1 {
		t.Fatal("retry changed object identity")
	}
	if err := s.SaveReplayPart(context.Background(), "project", p); err != nil || o.puts != 2 {
		t.Fatalf("ready retry uploaded again: %d %v", o.puts, err)
	}
	raw, err := decompressReplay(o.data[key])
	if err != nil || string(raw) != string(p.Data) {
		t.Fatalf("payload: %s %v", raw, err)
	}
}

func TestS3ConfigurationRejectsPathsAndUnknownProtocols(t *testing.T) {
	for _, endpoint := range []string{"localhost:9000", "ftp://localhost", "https://localhost/bucket"} {
		if _, err := NewS3(S3Config{Endpoint: endpoint}); err == nil {
			t.Errorf("accepted %s", endpoint)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := NewService(&memoryMetadata{}, nil).SaveReplayPart(ctx, "p", Part{}); err == nil {
		t.Fatal("missing S3 silently accepted")
	}
}
