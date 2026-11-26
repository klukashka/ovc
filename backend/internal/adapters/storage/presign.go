package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"ovc/backend/internal/core"
)

const PresignedReadLifetime = 5 * time.Minute

var (
	ErrPresignRefused  = errors.New("the storage presign produced an address that is not a plain read")
	ErrPresignLifetime = errors.New("a presigned download lasts more than nothing and at most the presigned read lifetime")
)

type Presigner struct {
	client *s3.PresignClient
	bucket string
}

func NewPresigner(settings core.StorageSettings, endpoint string) *Presigner {
	client := s3.New(s3.Options{
		Region:                     settings.Region,
		BaseEndpoint:               aws.String(endpoint),
		UsePathStyle:               true,
		Credentials:                credentials.NewStaticCredentialsProvider(settings.AccessKey.Reveal(), settings.SecretKey.Reveal(), ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &Presigner{client: s3.NewPresignClient(client, s3.WithPresignExpires(PresignedReadLifetime)), bucket: settings.Bucket}
}

func (p *Presigner) PresignRead(ctx context.Context, key string) (core.Secret, error) {
	signed, err := p.client.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(p.bucket), Key: aws.String(key)})
	if err != nil {
		return core.Secret{}, fmt.Errorf("presign a storage read: %w", err)
	}
	if signed.Method != http.MethodGet || signed.URL == "" || !onlyHostSigned(signed.SignedHeader) {
		return core.Secret{}, ErrPresignRefused
	}
	return core.NewSecret(signed.URL), nil
}

func onlyHostSigned(headers http.Header) bool {
	for name := range headers {
		if !strings.EqualFold(name, "Host") {
			return false
		}
	}
	return true
}

func (p *Presigner) PresignDownload(ctx context.Context, key string, contentType string, disposition string, lifetime time.Duration) (core.Secret, error) {
	if lifetime <= 0 || lifetime > PresignedReadLifetime {
		return core.Secret{}, ErrPresignLifetime
	}
	input := &s3.GetObjectInput{
		Bucket:                     aws.String(p.bucket),
		Key:                        aws.String(key),
		ResponseContentType:        aws.String(contentType),
		ResponseContentDisposition: aws.String(disposition),
	}
	signed, err := p.client.PresignGetObject(ctx, input, s3.WithPresignExpires(lifetime))
	if err != nil {
		return core.Secret{}, fmt.Errorf("presign a storage download: %w", err)
	}
	if signed.Method != http.MethodGet || signed.URL == "" || !onlyHostSigned(signed.SignedHeader) {
		return core.Secret{}, ErrPresignRefused
	}
	return core.NewSecret(signed.URL), nil
}
