package replay

import (
	"bytes"
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"net/url"
)

type S3Config struct{ Endpoint, Bucket, AccessKey, SecretKey, Region string }
type S3 struct {
	client *minio.Client
	bucket string
}

func NewS3(cfg S3Config) (*S3, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("S3 endpoint must be an http(s) origin")
	}
	client, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: u.Scheme == "https", Region: cfg.Region})
	if err != nil {
		return nil, err
	}
	return &S3{client, cfg.Bucket}, nil
}

func (s *S3) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}
func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: "application/gzip"})
	return err
}
func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	raw, err := io.ReadAll(io.LimitReader(object, MaxPart+1025))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxPart+1024 {
		return nil, ErrQuota
	}
	return raw, nil
}
func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
