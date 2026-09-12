// Package pgvector builds Postgres vector literals for use with the pgvector
// extension. The project's pool runs in simple-protocol mode, so queries cast
// the literal to the vector type explicitly (e.g. `embedding <=> $1::vector`).
package pgvector

import (
	"strconv"
	"strings"
)

// Literal renders embeddings as a pgvector-compatible string literal, e.g.
// "[0.1,0.2,0.3]".
func Literal(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}