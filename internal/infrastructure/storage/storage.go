package storage

import (
	"context"
	"io"
)

type Object struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
}

type Client interface {
	Upload(ctx context.Context, path string, contentType string, reader io.Reader) (string, error)
	Get(ctx context.Context, path string) (*Object, error)
	Delete(ctx context.Context, path string) error
}
