package berita

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
)

func TestImageRequestScheme(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		proto   string
		tls     bool
		want    string
	}{
		{name: "public host defaults to https", host: "jhicbe-3svk08cr.b4a.run", want: "https"},
		{name: "https forwarded proto wins", host: "jhicbe-3svk08cr.b4a.run", proto: "https", want: "https"},
		{name: "stale http forwarded proto ignored for public host", host: "jhicbe-3svk08cr.b4a.run", proto: "http", want: "https"},
		{name: "direct TLS", host: "example.com", tls: true, want: "https"},
		{name: "localhost stays http", host: "localhost:8080", want: "http"},
		{name: "127.0.0.1 stays http", host: "127.0.0.1:8080", want: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/", nil)
			req.Host = tt.host
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if got := imageRequestScheme(req); got != tt.want {
				t.Errorf("imageRequestScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsValidContentKey(t *testing.T) {
	tests := []struct {
		name    string
		berita  id.ID
		key     string
		wantErr bool
	}{
		{name: "valid key", berita: id.ID(1), key: "berita/1/content/abc.png"},
		{name: "nested content key", berita: id.ID(1), key: "berita/1/content/sub/abc.png"},
		{name: "other berita rejected", berita: id.ID(1), key: "berita/2/content/abc.png", wantErr: true},
		{name: "cover image rejected", berita: id.ID(1), key: "berita/1/abc.png", wantErr: true},
		{name: "signed url rejected", berita: id.ID(1), key: "https://bucket/berita/1/content/abc.png", wantErr: true},
		{name: "absolute path rejected", berita: id.ID(1), key: "/berita/1/content/abc.png", wantErr: true},
		{name: "traversal rejected", berita: id.ID(1), key: "berita/1/content/../abc.png", wantErr: true},
		{name: "query string rejected", berita: id.ID(1), key: "berita/1/content/abc.png?x=1", wantErr: true},
		{name: "empty key rejected", berita: id.ID(1), key: "", wantErr: true},
		{name: "key with space rejected", berita: id.ID(1), key: "berita/1/content/a b.png", wantErr: true},
		{name: "content dir itself rejected", berita: id.ID(1), key: "berita/1/content/", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidContentKey(tt.berita, tt.key)
			if (got) == tt.wantErr {
				t.Errorf("isValidContentKey() = %v, wantErr = %v", got, tt.wantErr)
			}
		})
	}
}

func TestExtractObjectPath(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name:    "simple path",
			url:     "https://jhic-berita-images.s3.eu-central-003.backblazeb2.com/berita/abc/def.jpg",
			want:    "berita/abc/def.jpg",
			wantErr: false,
		},
		{
			name:    "root object",
			url:     "https://bucket.endpoint.com/object.png",
			want:    "object.png",
			wantErr: false,
		},
		{
			name:    "nested path",
			url:     "https://bucket.endpoint.com/a/b/c/d.png",
			want:    "a/b/c/d.png",
			wantErr: false,
		},
		{
			name:    "query string stripped",
			url:     "https://bucket.endpoint.com/berita/1/content/x.png?X-Amz-Signature=abc",
			want:    "berita/1/content/x.png",
			wantErr: false,
		},
		{
			name:    "localhost with port",
			url:     "http://localhost:8080/berita/1/x.png",
			want:    "berita/1/x.png",
			wantErr: false,
		},
		{
			name:    "invalid url",
			url:     "://invalid",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractObjectPath(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractObjectPath() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractObjectPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
