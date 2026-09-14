package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func newTestClient(t *testing.T) *LocalClient {
	t.Helper()
	c, err := NewLocalClient(LocalConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewLocalClient: %v", err)
	}
	return c
}

func TestLocalClient_UploadGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	data := append(append([]byte{}, pngMagic...), []byte("fake-png-body")...)
	path, err := client.Upload(ctx, "berita/b1/photo.png", "image/png", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if path != "berita/b1/photo.png" {
		t.Fatalf("path = %q, want %q", path, "berita/b1/photo.png")
	}

	obj, err := client.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer obj.Body.Close()

	if obj.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", obj.ContentType)
	}
	if obj.ContentLength != int64(len(data)) {
		t.Errorf("ContentLength = %d, want %d", obj.ContentLength, len(data))
	}
	got, err := io.ReadAll(obj.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("body mismatch: got %v want %v", got, data)
	}
}

func TestLocalClient_UploadTrimsLeadingSlash(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	path, err := client.Upload(ctx, "/berita/b2/photo.png", "image/png", bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if path != "berita/b2/photo.png" {
		t.Fatalf("path = %q, want %q", path, "berita/b2/photo.png")
	}
	if _, err := client.Get(ctx, path); err != nil {
		t.Fatalf("Get after trimmed upload: %v", err)
	}
}

func TestLocalClient_GetMissingObject(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	_, err := client.Get(ctx, "berita/nope.png")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Get error = %v, want os.ErrNotExist", err)
	}
}

func TestLocalClient_Delete(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	path, err := client.Upload(ctx, "berita/b3/x.png", "image/png", bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if err := client.Delete(ctx, path); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := client.Get(ctx, path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Get after delete error = %v, want os.ErrNotExist", err)
	}

	if err := client.Delete(ctx, path); err != nil {
		t.Fatalf("Delete missing is idempotent, got %v", err)
	}
}

func TestLocalClient_PathTraversalRejected(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	evil := []string{
		"../../etc/passwd",
		"../x.png",
		"berita/../../../x.png",
	}
	for _, p := range evil {
		if _, err := client.Upload(ctx, p, "image/png", strings.NewReader("x")); err == nil {
			t.Errorf("Upload(%q) succeeded, want error", p)
		}
		if _, err := client.Get(ctx, p); err == nil {
			t.Errorf("Get(%q) succeeded, want error", p)
		}
	}
}

func TestNewLocalClient_RequiresDir(t *testing.T) {
	if _, err := NewLocalClient(LocalConfig{}); err == nil {
		t.Fatal("NewLocalClient with empty dir succeeded, want error")
	}
}