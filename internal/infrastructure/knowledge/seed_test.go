package knowledge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkText_SplitsLongText(t *testing.T) {
	text := strings.Repeat("line yang cukup panjang untuk melampaui batas\n", 100)
	chunks := chunkText(text, 200, 40)
	require.NotEmpty(t, chunks)
	for _, c := range chunks {
		require.LessOrEqual(t, len(c), 240)
	}
	require.True(t, len(chunks) > 1)
}

func TestChunkText_StartsNewChunkAtHeading(t *testing.T) {
	text := "# Judul\n\nparagraf satu.\n\n# Bagian Dua\n\nparagraf dua."
	chunks := chunkText(text, 1000, 0)
	require.Len(t, chunks, 2)
	require.Contains(t, chunks[0], "# Judul")
	require.Contains(t, chunks[0], "paragraf satu")
	require.Contains(t, chunks[1], "# Bagian Dua")
}

func TestChunkText_OverlapCarriesTail(t *testing.T) {
	text := strings.Repeat("abcdefghij ", 60)
	chunks := chunkText(text, 100, 40)
	require.True(t, len(chunks) > 1)
	require.Contains(t, chunks[1], chunks[0][len(chunks[0])-40:])
}

func TestSlugify(t *testing.T) {
	require.Equal(t, "selayang-pandang", slugify("selayang pandang"))
	require.Equal(t, "ppdb", slugify("PPDB"))
	require.Equal(t, "ppdb-2026", slugify("PPDB-2026"))
}

func TestDocumentTitle(t *testing.T) {
	require.Equal(t, "Judul", documentTitle("# Judul\n\nIsi.", "slug"))
	require.Equal(t, "slug", documentTitle("Isi tanpa judul.", "slug"))
}