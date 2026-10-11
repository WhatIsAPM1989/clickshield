package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type Chunk struct {
	Part
	Key, Checksum string
	Ready         bool
}
type Objects interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}
type Metadata interface {
	Reserve(context.Context, string, Chunk) (bool, error)
	MarkReady(context.Context, Chunk) error
	Load(context.Context, string) (*time.Time, []Chunk, error)
	Delete(context.Context, string) error
}
type Service struct {
	repository Metadata
	objects    Objects
}

func NewService(repository Metadata, objects Objects) *Service { return &Service{repository, objects} }
func (s *Service) Configured() bool                            { return s != nil && s.objects != nil }

func checksum(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func (s *Service) SaveReplayPart(ctx context.Context, project string, p Part) error {
	if s.objects == nil {
		return fmt.Errorf("replay object storage is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	digest := checksum(p.Data)
	chunk := Chunk{Part: p, Checksum: digest, Key: fmt.Sprintf("replay/%s/%s/%s/%d/%d-%s.gz", project, p.SessionID, p.PageID, p.Sequence, p.PartIndex, digest)}
	ready, err := s.repository.Reserve(ctx, project, chunk)
	if err != nil || ready {
		return err
	}
	compressed, err := compressReplay(p.Data)
	if err != nil {
		return err
	}
	if err = s.objects.Put(ctx, chunk.Key, compressed); err != nil {
		return err
	}
	return s.repository.MarkReady(ctx, chunk)
}

func (s *Service) Delete(ctx context.Context, id string) error { return s.repository.Delete(ctx, id) }
