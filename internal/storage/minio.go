package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOConfig contains MinIO connection settings
type MinIOConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	BucketName      string
}

// MinIOClient wraps the MinIO client
type MinIOClient struct {
	client     *minio.Client
	bucketName string
}

// NewMinIOClient creates a new MinIO client
func NewMinIOClient(cfg MinIOConfig) (*MinIOClient, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating minio client: %w", err)
	}

	return &MinIOClient{
		client:     client,
		bucketName: cfg.BucketName,
	}, nil
}

// EnsureBucket creates the bucket if it doesn't exist
func (m *MinIOClient) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucketName)
	if err != nil {
		return fmt.Errorf("checking bucket existence: %w", err)
	}

	if !exists {
		err = m.client.MakeBucket(ctx, m.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}
	}

	return nil
}

// UploadScreenshot uploads a screenshot and returns the S3 URI
func (m *MinIOClient) UploadScreenshot(ctx context.Context, bucket, key string, data []byte) (string, error) {
	reader := bytes.NewReader(data)

	contentType := "image/jpeg"
	if len(key) > 4 && key[len(key)-4:] == ".png" {
		contentType = "image/png"
	}

	_, err := m.client.PutObject(ctx, bucket, key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("uploading object: %w", err)
	}

	// Return S3-style URI
	return fmt.Sprintf("s3://%s/%s", bucket, key), nil
}

// Upload uploads any file to MinIO
func (m *MinIOClient) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	reader := bytes.NewReader(data)

	_, err := m.client.PutObject(ctx, m.bucketName, key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("uploading object: %w", err)
	}

	return fmt.Sprintf("s3://%s/%s", m.bucketName, key), nil
}

// UploadJSON uploads JSON data to MinIO
func (m *MinIOClient) UploadJSON(ctx context.Context, key string, data []byte) (string, error) {
	return m.Upload(ctx, key, data, "application/json")
}

// Download downloads a file from MinIO
func (m *MinIOClient) Download(ctx context.Context, key string) ([]byte, error) {
	obj, err := m.client.GetObject(ctx, m.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting object: %w", err)
	}
	defer obj.Close()

	return io.ReadAll(obj)
}

// Delete deletes a file from MinIO
func (m *MinIOClient) Delete(ctx context.Context, key string) error {
	return m.client.RemoveObject(ctx, m.bucketName, key, minio.RemoveObjectOptions{})
}

// GetPresignedURL returns a presigned URL for downloading
func (m *MinIOClient) GetPresignedURL(ctx context.Context, key string) (string, error) {
	url, err := m.client.PresignedGetObject(ctx, m.bucketName, key, 0, nil)
	if err != nil {
		return "", fmt.Errorf("generating presigned URL: %w", err)
	}
	return url.String(), nil
}

// ListObjects lists objects with a given prefix
func (m *MinIOClient) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	var keys []string

	objectCh := m.client.ListObjects(ctx, m.bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, object.Err
		}
		keys = append(keys, object.Key)
	}

	return keys, nil
}
