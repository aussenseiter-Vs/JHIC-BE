package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LocalClient stores objects as plain files under a root directory and serves
// them straight through the API's image proxy (no presigning, no object store).
type LocalClient struct {
	dir string
}

type LocalConfig struct {
	Dir string
}

func NewLocalClient(cfg LocalConfig) (*LocalClient, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("local storage: dir is required")
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("local storage: create dir: %w", err)
	}
	return &LocalClient{dir: cfg.Dir}, nil
}

// resolvePath maps a bare object path onto the storage dir, rejecting any path
// that would escape the root (traversal, absolute paths).
func (c *LocalClient) resolvePath(objectPath string) (string, error) {
	if objectPath == "" {
		return "", fmt.Errorf("local storage: empty object path")
	}
	clean := strings.TrimPrefix(objectPath, "/")
	local := filepath.Join(c.dir, filepath.FromSlash(clean))
	if c.dir != "." {
		rel, err := filepath.Rel(c.dir, local)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("local storage: object path escapes root: %q", objectPath)
		}
	}
	return local, nil
}

func (c *LocalClient) Upload(ctx context.Context, objectPath, contentType string, reader io.Reader) (string, error) {
	local, err := c.resolvePath(objectPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return "", fmt.Errorf("local storage: mkdir: %w", err)
	}

	f, err := os.Create(local)
	if err != nil {
		return "", fmt.Errorf("local storage: create: %w", err)
	}
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		return "", fmt.Errorf("local storage: write: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("local storage: close: %w", err)
	}

	return strings.TrimPrefix(objectPath, "/"), nil
}

func (c *LocalClient) Get(ctx context.Context, objectPath string) (*Object, error) {
	local, err := c.resolvePath(objectPath)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(local)
	if err != nil {
		return nil, err
	}

	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("local storage: stat: %w", err)
	}

	ct, err := detectContentType(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, fmt.Errorf("local storage: seek: %w", err)
	}

	return &Object{
		Body:          f,
		ContentType:   ct,
		ContentLength: st.Size(),
	}, nil
}

func (c *LocalClient) Delete(ctx context.Context, objectPath string) error {
	local, err := c.resolvePath(objectPath)
	if err != nil {
		return err
	}
	if err := os.Remove(local); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("local storage: delete: %w", err)
	}
	return nil
}

func detectContentType(f *os.File) (string, error) {
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("local storage: read head: %w", err)
	}
	if n == 0 {
		return "application/octet-stream", nil
	}
	return http.DetectContentType(buf[:n]), nil
}