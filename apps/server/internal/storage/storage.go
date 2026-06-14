// Package storage 复刻 apps/api/src/services/object-storage.ts：aws-sdk-go-v2 + path-style，
// 与原 aws-sdk-v3 保持 presigned URL 同形（http://endpoint/<bucket>/<key>?X-Amz-...）。
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	appconfig "medoffice/server/internal/config"
)

type Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func New(ctx context.Context, cfg appconfig.Storage) (*Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(orDefault(cfg.AccessKey, "minioadmin"), orDefault(cfg.SecretKey, "minioadmin"), ""),
		),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	return &Storage{client: client, presign: s3.NewPresignClient(client), bucket: cfg.Bucket}, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func (s *Storage) ensureBucket(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket}); err == nil {
		return nil
	}
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket})
	var owned *types.BucketAlreadyOwnedByYou
	var exists *types.BucketAlreadyExists
	if errors.As(err, &owned) || errors.As(err, &exists) {
		return nil
	}
	return err
}

func (s *Storage) Put(ctx context.Context, key string, body []byte, contentType string) error {
	if err := s.ensureBucket(ctx); err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &key,
		Body:        bytes.NewReader(body),
		ContentType: &contentType,
	})
	return err
}

func (s *Storage) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return err
}

type Head struct {
	Size        int64
	ContentType string
}

func (s *Storage) HeadObject(ctx context.Context, key string) (Head, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return Head{}, err
	}
	h := Head{}
	if out.ContentLength != nil {
		h.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		h.ContentType = *out.ContentType
	}
	return h, nil
}

func (s *Storage) PresignedURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	if expires <= 0 {
		expires = 300 * time.Second
	}
	req, err := s.presign.PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: &s.bucket, Key: &key},
		s3.WithPresignExpires(expires),
	)
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func ObjectKeyForVersion(tenantID, documentID, versionID string) string {
	return tenantID + "/" + documentID + "/" + versionID
}

func ComputeFileHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
