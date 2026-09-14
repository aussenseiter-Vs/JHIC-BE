package berita

import (
	"errors"
	"strings"
	"testing"
)

func TestExtractImageKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "single internal key",
			content: `![photo](berita/1/content/abc.png)`,
			want:    []string{"berita/1/content/abc.png"},
		},
		{
			name:    "multiple internal keys with surrounding markdown",
			content: "# Title\n\nFirst paragraph.\n\n![a](berita/2/content/x.png)\n\nSecond ![b](berita/2/content/y.jpg).",
			want:    []string{"berita/2/content/x.png", "berita/2/content/y.jpg"},
		},
		{
			name:    "no images",
			content: "# Title\n\nJust text.",
			want:    nil,
		},
		{
			name:    "external image ignored",
			content: `![logo](https://example.com/logo.png)`,
			want:    nil,
		},
		{
			name:    "non-image link ignored",
			content: `[read more](berita/1/content/abc.png)`,
			want:    nil,
		},
		{
			name:    "image with title suffix",
			content: `![photo](berita/1/content/abc.png "caption")`,
			want:    []string{"berita/1/content/abc.png"},
		},
		{
			name:    "empty content",
			content: "",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractImageKeys(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("extractImageKeys() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractImageKeys() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestNormalizeImageRefs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "bare keys left unchanged",
			content: `![a](berita/1/content/x.png)`,
			want:    `![a](berita/1/content/x.png)`,
		},
		{
			name:    "signed url normalized to key",
			content: `![a](https://bucket.endpoint.com/berita/1/content/x.png?X-Amz-Signature=abc)`,
			want:    `![a](berita/1/content/x.png)`,
		},
		{
			name:    "bucket-prefixed url not rewritten",
			content: `![a](https://s3.eu-central-003.backblazeb2.com/jhic-berita-images/berita/1/content/x.png?X-Amz-Signature=abc&X-Amz-Date=20260803T071952Z)`,
			want:    `![a](https://s3.eu-central-003.backblazeb2.com/jhic-berita-images/berita/1/content/x.png?X-Amz-Signature=abc&X-Amz-Date=20260803T071952Z)`,
		},
		{
			name:    "external url untouched",
			content: `![a](https://example.com/photo.png)`,
			want:    `![a](https://example.com/photo.png)`,
		},
		{
			name:    "mixed refs",
			content: `![signed](https://bucket.endpoint.com/berita/2/content/a.jpg)` + " " + `![ext](https://example.com/b.jpg)` + " " + `![key](berita/2/content/c.png)`,
			want:    `![signed](berita/2/content/a.jpg)` + " " + `![ext](https://example.com/b.jpg)` + " " + `![key](berita/2/content/c.png)`,
		},
		{
			name:    "title preserved",
			content: `![a](https://bucket.endpoint.com/berita/1/content/x.png "cap")`,
			want:    `![a](berita/1/content/x.png "cap")`,
		},
		{
			name:    "no images",
			content: "# Title\n\nBody.",
			want:    "# Title\n\nBody.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeImageRefs(tt.content)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeImageRefs() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("normalizeImageRefs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveImageRefs(t *testing.T) {
	sign := func(key string) (string, error) {
		return "https://cdn.example.com/" + key + "?sig=1", nil
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "internal key resolved",
			content: `![a](berita/1/content/x.png)`,
			want:    `![a](https://cdn.example.com/berita/1/content/x.png?sig=1)`,
		},
		{
			name:    "external url untouched",
			content: `![a](https://example.com/photo.png)`,
			want:    `![a](https://example.com/photo.png)`,
		},
		{
			name:    "no images",
			content: "# Title\n\nBody.",
			want:    "# Title\n\nBody.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveImageRefs(tt.content, sign)
			if got != tt.want {
				t.Errorf("resolveImageRefs() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("signing error leaves ref unchanged", func(t *testing.T) {
		content := `![a](berita/1/content/x.png)`
		got := resolveImageRefs(content, func(string) (string, error) {
			return "", errors.New("presign failed")
		})
		if got != content {
			t.Errorf("resolveImageRefs() = %q, want unchanged %q", got, content)
		}
	})

	t.Run("key with title keeps title", func(t *testing.T) {
		got := resolveImageRefs(`![a](berita/1/content/x.png "cap")`, sign)
		want := `![a](https://cdn.example.com/berita/1/content/x.png?sig=1 "cap")`
		if got != want {
			t.Errorf("resolveImageRefs() = %q, want %q", got, want)
		}
	})
}

func TestIsInternalImageKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"berita/1/content/x.png", true},
		{"berita/1/x.png", true},
		{"berita/", true},
		{"https://bucket/berita/1/content/x.png", false},
		{"https://example.com/x.png", false},
		{"images/berita/1/content/x.png", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isInternalImageKey(tt.key); got != tt.want {
			t.Errorf("isInternalImageKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestExtractImageKeysLargeContent(t *testing.T) {
	content := strings.Repeat("# Heading\n\nParagraph.\n\n", 50) + `![x](berita/1/content/large.png)`
	keys := extractImageKeys(content)
	if len(keys) != 1 || keys[0] != "berita/1/content/large.png" {
		t.Fatalf("extractImageKeys() = %v", keys)
	}
}
