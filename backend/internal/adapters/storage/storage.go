package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"ovc/backend/internal/core"
)

var (
	ErrObjectNotFound      = errors.New("the storage object does not exist")
	ErrUploadNotFound      = errors.New("the multipart upload does not exist")
	ErrInvalidPart         = errors.New("a completed part does not match an uploaded part")
	ErrRangeNotSatisfiable = errors.New("the requested range starts outside the storage object")
	ErrMalformedResponse   = errors.New("the storage server answered without a required field")
)

const MinimumPartSize = 5 << 20

type MultipartUploadID string

type PartNumber int32

type ETag string

type CompletedPart struct {
	Number PartNumber
	ETag   ETag
}

type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	ReadRange(ctx context.Context, key string, offset int64, buffer []byte) (int, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
	CreateMultipartUpload(ctx context.Context, key string, contentType string) (MultipartUploadID, error)
	UploadPart(ctx context.Context, key string, upload MultipartUploadID, number PartNumber, data []byte) (CompletedPart, error)
	CompleteMultipartUpload(ctx context.Context, key string, upload MultipartUploadID, parts []CompletedPart) error
	AbortMultipartUpload(ctx context.Context, key string, upload MultipartUploadID) error
}

type RangeReader struct {
	ctx    context.Context
	store  Store
	object string
}

func NewRangeReader(ctx context.Context, store Store, object string) *RangeReader {
	return &RangeReader{ctx: ctx, store: store, object: object}
}

func (r *RangeReader) ReadAt(buffer []byte, offset int64) (int, error) {
	return r.store.ReadRange(r.ctx, r.object, offset, buffer)
}

const (
	codeNoSuchUpload     = "NoSuchUpload"
	codeInvalidPart      = "InvalidPart"
	codeInvalidPartOrder = "InvalidPartOrder"
	codeEntityTooSmall   = "EntityTooSmall"
)

type apiError interface {
	ErrorCode() string
}

type statusError interface {
	HTTPStatusCode() int
}

func hasCode(err error, codes ...string) bool {
	var coded apiError
	return errors.As(err, &coded) && slices.Contains(codes, coded.ErrorCode())
}

func hasStatus(err error, status int) bool {
	var answered statusError
	return errors.As(err, &answered) && answered.HTTPStatusCode() == status
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

func wrapped(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func (s *S3) ReadRange(ctx context.Context, key string, offset int64, buffer []byte) (int, error) {
	if offset < 0 {
		return 0, ErrRangeNotSatisfiable
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	requested := "bytes=" + strconv.FormatInt(offset, 10) + "-" + strconv.FormatInt(offset+int64(len(buffer))-1, 10)
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Range: aws.String(requested)})
	var missing *types.NoSuchKey
	if errors.As(err, &missing) {
		return 0, ErrObjectNotFound
	}
	if hasStatus(err, http.StatusRequestedRangeNotSatisfiable) {
		return 0, ErrRangeNotSatisfiable
	}
	if err != nil {
		return 0, fmt.Errorf("read a storage object range: %w", err)
	}
	served, err := servedLength(output.ContentRange, offset, len(buffer))
	if err != nil {
		return 0, errors.Join(err, wrapped("close a storage object range", output.Body.Close()))
	}
	_, readErr := io.ReadFull(output.Body, buffer[:served])
	closeErr := output.Body.Close()
	if err := errors.Join(wrapped("read a storage object range", readErr), wrapped("close a storage object range", closeErr)); err != nil {
		return 0, err
	}
	if served < len(buffer) {
		return served, io.EOF
	}
	return served, nil
}

func servedLength(contentRange *string, offset int64, requested int) (int, error) {
	if contentRange == nil {
		return 0, fmt.Errorf("read a storage object range: %w", ErrMalformedResponse)
	}
	var first, last, total int64
	if _, err := fmt.Sscanf(*contentRange, "bytes %d-%d/%d", &first, &last, &total); err != nil {
		return 0, fmt.Errorf("read a storage object range: %w", ErrMalformedResponse)
	}
	if first != offset || offset >= total || last < first || last >= total {
		return 0, fmt.Errorf("read a storage object range: %w", ErrMalformedResponse)
	}
	served := last - first + 1
	if served != min(int64(requested), total-offset) {
		return 0, fmt.Errorf("read a storage object range: %w", ErrMalformedResponse)
	}
	return int(served), nil
}

func (s *S3) CreateMultipartUpload(ctx context.Context, key string, contentType string) (MultipartUploadID, error) {
	output, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), ContentType: aws.String(contentType),
	}, singleAttempt)
	if err != nil {
		return "", fmt.Errorf("create a multipart upload: %w", err)
	}
	if output.UploadId == nil || *output.UploadId == "" {
		return "", fmt.Errorf("create a multipart upload: %w", ErrMalformedResponse)
	}
	return MultipartUploadID(*output.UploadId), nil
}

func (s *S3) UploadPart(
	ctx context.Context, key string, upload MultipartUploadID, number PartNumber, data []byte,
) (CompletedPart, error) {
	output, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: aws.String(string(upload)),
		PartNumber: aws.Int32(int32(number)), Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data))),
	})
	if hasCode(err, codeNoSuchUpload) {
		return CompletedPart{}, ErrUploadNotFound
	}
	if err != nil {
		return CompletedPart{}, fmt.Errorf("upload a multipart upload part: %w", err)
	}
	if output.ETag == nil || *output.ETag == "" {
		return CompletedPart{}, fmt.Errorf("upload a multipart upload part: %w", ErrMalformedResponse)
	}
	return CompletedPart{Number: number, ETag: ETag(*output.ETag)}, nil
}

func (s *S3) CompleteMultipartUpload(ctx context.Context, key string, upload MultipartUploadID, parts []CompletedPart) error {
	if len(parts) == 0 {
		return ErrInvalidPart
	}
	completed := make([]types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		completed = append(completed, types.CompletedPart{PartNumber: aws.Int32(int32(part.Number)), ETag: aws.String(string(part.ETag))})
	}
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: aws.String(string(upload)),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed},
	}, singleAttempt)
	if hasCode(err, codeNoSuchUpload) {
		return ErrUploadNotFound
	}
	if hasCode(err, codeInvalidPart, codeInvalidPartOrder, codeEntityTooSmall) {
		return ErrInvalidPart
	}
	if err != nil {
		return fmt.Errorf("complete a multipart upload: %w", err)
	}
	return nil
}

func (s *S3) AbortMultipartUpload(ctx context.Context, key string, upload MultipartUploadID) error {
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: aws.String(string(upload)),
	})
	if err == nil || hasCode(err, codeNoSuchUpload) {
		return nil
	}
	return fmt.Errorf("abort a multipart upload: %w", err)
}

func (s *S3) IncompleteUploads(ctx context.Context, key string) ([]MultipartUploadID, error) {
	input := &s3.ListMultipartUploadsInput{Bucket: aws.String(s.bucket), Prefix: aws.String(key)}
	var uploads []MultipartUploadID
	for {
		output, err := s.client.ListMultipartUploads(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list incomplete multipart uploads: %w", err)
		}
		for _, upload := range output.Uploads {
			if upload.Key != nil && *upload.Key == key && upload.UploadId != nil {
				uploads = append(uploads, MultipartUploadID(*upload.UploadId))
			}
		}
		if output.IsTruncated == nil || !*output.IsTruncated {
			return uploads, nil
		}
		if !advanced(input.KeyMarker, input.UploadIdMarker, output.NextKeyMarker, output.NextUploadIdMarker) {
			return nil, fmt.Errorf("list incomplete multipart uploads: %w", ErrMalformedResponse)
		}
		input.KeyMarker, input.UploadIdMarker = output.NextKeyMarker, output.NextUploadIdMarker
	}
}

func advanced(keyMarker, uploadMarker, nextKeyMarker, nextUploadMarker *string) bool {
	if nextKeyMarker == nil || nextUploadMarker == nil {
		return false
	}
	return aws.ToString(keyMarker) != *nextKeyMarker || aws.ToString(uploadMarker) != *nextUploadMarker
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
	if err != nil && !errors.As(err, &owned) && !errors.As(err, &exists) {
		return fmt.Errorf("create the storage bucket: %w", err)
	}
	_, err = s.client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(s.bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: []types.LifecycleRule{{
			ID: aws.String(incompleteUploadsRule), Status: types.ExpirationStatusEnabled, Filter: &types.LifecycleRuleFilter{Prefix: aws.String("")},
			AbortIncompleteMultipartUpload: &types.AbortIncompleteMultipartUpload{DaysAfterInitiation: aws.Int32(incompleteUploadDays)},
		}}},
	})
	return wrapped("set the storage bucket lifecycle", err)
}

const (
	incompleteUploadsRule = "abort-incomplete-multipart-uploads"
	incompleteUploadDays  = 1
)

func (s *S3) DeleteEmptyBucket(ctx context.Context) error {
	if _, err := s.client.DeleteBucketLifecycle(ctx, &s3.DeleteBucketLifecycleInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("delete the storage bucket lifecycle: %w", err)
	}
	_, err := s.client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(s.bucket)})
	return wrapped("delete the storage bucket", err)
}

func (s *S3) IncompleteUploadDays(ctx context.Context) (int32, bool, error) {
	output, err := s.client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		return 0, false, fmt.Errorf("read the storage bucket lifecycle: %w", err)
	}
	for _, rule := range output.Rules {
		if aws.ToString(rule.ID) == incompleteUploadsRule && rule.Status == types.ExpirationStatusEnabled && rule.AbortIncompleteMultipartUpload != nil {
			return aws.ToInt32(rule.AbortIncompleteMultipartUpload.DaysAfterInitiation), true, nil
		}
	}
	return 0, false, nil
}

type memoryUpload struct {
	object string
	parts  map[PartNumber][]byte
}

type Memory struct {
	mutex     sync.Mutex
	objects   map[string][]byte
	uploads   map[MultipartUploadID]*memoryUpload
	completed map[MultipartUploadID]string
	created   int
}

func NewMemory() *Memory {
	return &Memory{
		objects: map[string][]byte{}, uploads: map[MultipartUploadID]*memoryUpload{}, completed: map[MultipartUploadID]string{},
	}
}

func memoryETag(data []byte) ETag {
	sum := sha256.Sum256(data)
	return ETag(hex.EncodeToString(sum[:]))
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

func (m *Memory) ReadRange(_ context.Context, key string, offset int64, buffer []byte) (int, error) {
	if offset < 0 {
		return 0, ErrRangeNotSatisfiable
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	data, found := m.objects[key]
	if !found {
		return 0, ErrObjectNotFound
	}
	if offset >= int64(len(data)) {
		return 0, ErrRangeNotSatisfiable
	}
	read := copy(buffer, data[offset:])
	if read < len(buffer) {
		return read, io.EOF
	}
	return read, nil
}

func (m *Memory) CreateMultipartUpload(_ context.Context, key string, _ string) (MultipartUploadID, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.created++
	upload := MultipartUploadID("memory-upload-" + strconv.Itoa(m.created))
	m.uploads[upload] = &memoryUpload{object: key, parts: map[PartNumber][]byte{}}
	return upload, nil
}

func (m *Memory) UploadPart(
	_ context.Context, key string, upload MultipartUploadID, number PartNumber, data []byte,
) (CompletedPart, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	pending, found := m.uploads[upload]
	if !found || pending.object != key {
		return CompletedPart{}, ErrUploadNotFound
	}
	pending.parts[number] = bytes.Clone(data)
	return CompletedPart{Number: number, ETag: memoryETag(data)}, nil
}

func (m *Memory) CompleteMultipartUpload(_ context.Context, key string, upload MultipartUploadID, parts []CompletedPart) error {
	if len(parts) == 0 {
		return ErrInvalidPart
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if completedObject, completed := m.completed[upload]; completed && completedObject == key {
		return nil
	}
	pending, found := m.uploads[upload]
	if !found || pending.object != key {
		return ErrUploadNotFound
	}
	var object []byte
	for index, part := range parts {
		data, uploaded := pending.parts[part.Number]
		if !uploaded || memoryETag(data) != part.ETag || (index > 0 && parts[index-1].Number >= part.Number) {
			return ErrInvalidPart
		}
		if index < len(parts)-1 && len(data) < MinimumPartSize {
			return ErrInvalidPart
		}
		object = append(object, data...)
	}
	m.objects[key] = object
	delete(m.uploads, upload)
	m.completed[upload] = key
	return nil
}

func (m *Memory) AbortMultipartUpload(_ context.Context, key string, upload MultipartUploadID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if pending, found := m.uploads[upload]; found && pending.object == key {
		delete(m.uploads, upload)
	}
	return nil
}

func (m *Memory) IncompleteUploads(_ context.Context, key string) ([]MultipartUploadID, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	var uploads []MultipartUploadID
	for upload, pending := range m.uploads {
		if pending.object == key {
			uploads = append(uploads, upload)
		}
	}
	slices.Sort(uploads)
	return uploads, nil
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

func singleAttempt(options *s3.Options) {
	options.RetryMaxAttempts = 1
}
