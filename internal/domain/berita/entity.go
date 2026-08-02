package berita

import (
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
)

type Berita struct {
	ID            id.ID     `json:"id"`
	AuthorID      id.ID     `json:"author_id"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	ImageURL      string    `json:"image_url,omitempty"`
	IsAchievement bool      `json:"is_achievement"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
