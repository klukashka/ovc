package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"ovc/backend/internal/core"
)

var ErrObjectNotFound = errors.New("the storage object does not exist")

type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

const (
	dialTimeout           = 5 * time.Second
	responseHeaderTimeout = 15 * time.Second
	retryAttempts         = 3
)

type S3 struct {
	client *s3.Client
	bucket string
}

func NewS3(settings core.StorageSettings) *S3 {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: dialTimeout}).DialContext,
		TLSHandshakeTimeout:   dialTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
	}
	client := s3.New(s3.Options{
		Region:                     settings.Region,
		BaseEndpoint:               aws.String(settings.Endpoint),
		UsePathStyle:               true,
		Credentials:                credentials.NewStaticCredentialsProvider(settings.AccessKey.Reveal(), settings.SecretKey.Reveal(), ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		HTTPClient:                 &http.Client{Transport: transport},
		RetryMaxAttempts:           retryAttempts,
	})
	return &S3{client: client, bucket: settings.Bucket}
}

func (s *S3) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))), ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put a storage object: %w", err)
	}
	return nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	var missing *types.NoSuchKey
	if errors.As(err, &missing) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open a storage object: %w", err)
	}
	return output.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}); err != nil {
		return fmt.Errorf("delete a storage object: %w", err)
	}
	return nil
}

func (s *S3) Ping(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("reach the storage bucket: %w", err)
	}
	return nil
}

func (s *S3) EnsureBucket(ctx context.Context) error {
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	var owned *types.BucketAlreadyOwnedByYou
	var exists *types.BucketAlreadyExists
	if err == nil || errors.As(err, &owned) || errors.As(err, &exists) {
		return nil
	}
	return fmt.Errorf("create the storage bucket: %w", err)
}

type Memory struct {
	mutex   sync.Mutex
	objects map[string][]byte
}

func NewMemory() *Memory {
	return &Memory{objects: map[string][]byte{}}
}

func (m *Memory) Put(_ context.Context, key string, data []byte, _ string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.objects[key] = bytes.Clone(data)
	return nil
}

func (m *Memory) Open(_ context.Context, key string) (io.ReadCloser, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	data, found := m.objects[key]
	if !found {
		return nil, ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(data))), nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *Memory) Ping(context.Context) error {
	return nil
}
