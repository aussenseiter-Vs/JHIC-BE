package berita

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/middleware"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/response"
	stor "github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/storage"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
)

type Handler struct {
	svc   *Service
	store stor.Client
}

func NewHandler(svc *Service, store stor.Client) *Handler {
	return &Handler{svc: svc, store: store}
}

func (h *Handler) Register(mux *http.ServeMux, authMw func(http.Handler) http.Handler, roleMw func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/berita", authMw(roleMw(http.HandlerFunc(h.Create))))
	mux.Handle("GET /api/v1/berita", http.HandlerFunc(h.List))
	mux.Handle("GET /api/v1/berita/{id}", http.HandlerFunc(h.Get))
	mux.Handle("PUT /api/v1/berita/{id}", authMw(roleMw(http.HandlerFunc(h.Update))))
	mux.Handle("DELETE /api/v1/berita/{id}", authMw(roleMw(http.HandlerFunc(h.Delete))))
	mux.Handle("POST /api/v1/berita/{id}/image", authMw(roleMw(http.HandlerFunc(h.UploadImage))))
	mux.Handle("POST /api/v1/berita/{id}/images", authMw(roleMw(http.HandlerFunc(h.UploadContentImage))))
	mux.Handle("DELETE /api/v1/berita/{id}/images", authMw(roleMw(http.HandlerFunc(h.DeleteContentImage))))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(id.ID)

	var input struct {
		Title         string `json:"title"`
		Content       string `json:"content"`
		IsAchievement bool   `json:"is_achievement"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Title == "" {
		response.Error(w, http.StatusBadRequest, "title is required")
		return
	}
	content, err := normalizeImageRefs(input.Content)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid content")
		return
	}

	b, err := h.svc.Create(r.Context(), userID, input.Title, content, input.IsAchievement)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.signArticle(r.Context(), b)
	response.JSON(w, http.StatusCreated, b)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range list {
		h.signArticle(r.Context(), &list[i])
	}
	response.JSON(w, http.StatusOK, list)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := id.Parse(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	b, err := h.svc.ByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		response.Error(w, http.StatusNotFound, "berita not found")
		return
	}
	h.signArticle(r.Context(), b)
	response.JSON(w, http.StatusOK, b)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(id.ID)
	id, err := id.Parse(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var input struct {
		Title         string `json:"title"`
		Content       string `json:"content"`
		IsAchievement bool   `json:"is_achievement"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	content, err := normalizeImageRefs(input.Content)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid content")
		return
	}

	b, err := h.svc.Update(r.Context(), id, userID, input.Title, content, input.IsAchievement)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.signArticle(r.Context(), b)
	response.JSON(w, http.StatusOK, b)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(id.ID)
	id, err := id.Parse(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	b, err := h.svc.ByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		response.Error(w, http.StatusNotFound, "berita not found")
		return
	}

	h.deleteObject(r.Context(), b.ImageURL)
	for _, key := range extractImageKeys(b.Content) {
		h.deleteObject(r.Context(), key)
	}

	if err := h.svc.Delete(r.Context(), id, userID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(id.ID)
	beritaID, err := id.Parse(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	objectPath, ok := h.uploadImage(w, r, fmt.Sprintf("berita/%s", beritaID))
	if !ok {
		return
	}

	b, err := h.svc.SetImage(r.Context(), beritaID, userID, objectPath)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	h.signArticle(r.Context(), b)
	response.JSON(w, http.StatusOK, map[string]string{"image_url": b.ImageURL})
}

func (h *Handler) UploadContentImage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(id.ID)
	beritaID, err := id.Parse(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	b, err := h.svc.ByID(r.Context(), beritaID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		response.Error(w, http.StatusNotFound, "berita not found")
		return
	}
	if b.AuthorID != userID {
		response.Error(w, http.StatusForbidden, "forbidden: not the author")
		return
	}

	objectPath, ok := h.uploadImage(w, r, fmt.Sprintf("berita/%s/content", beritaID))
	if !ok {
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"image_url": objectPath})
}

func (h *Handler) DeleteContentImage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(id.ID)
	beritaID, err := id.Parse(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	key := r.URL.Query().Get("key")
	if !isValidContentKey(beritaID, key) {
		response.Error(w, http.StatusBadRequest, "invalid key")
		return
	}

	b, err := h.svc.ByID(r.Context(), beritaID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		response.Error(w, http.StatusNotFound, "berita not found")
		return
	}
	if b.AuthorID != userID {
		response.Error(w, http.StatusForbidden, "forbidden: not the author")
		return
	}

	if err := h.store.Delete(r.Context(), key); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to delete image")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) uploadImage(w http.ResponseWriter, r *http.Request, prefix string) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "file too large or invalid multipart form")
		return "", false
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "missing image field")
		return "", false
	}
	defer file.Close()

	buf := make([]byte, 512)
	if _, err := file.Read(buf); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to read image")
		return "", false
	}
	file.Seek(0, io.SeekStart)

	detected := http.DetectContentType(buf)
	allowed := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}
	if !allowed[detected] {
		response.Error(w, http.StatusBadRequest, "unsupported image type: jpeg, png, gif, webp allowed")
		return "", false
	}

	ext := filepath.Ext(header.Filename)
	validExts := map[string]string{
		".jpg":  ".jpg",
		".jpeg": ".jpeg",
		".png":  ".png",
		".gif":  ".gif",
		".webp": ".webp",
	}
	if _, ok := validExts[strings.ToLower(ext)]; !ok {
		ext = ".jpg"
	}

	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to generate unique id")
		return "", false
	}
	objectPath := fmt.Sprintf("%s/%s%s", prefix, hex.EncodeToString(uuid), strings.ToLower(ext))

	if _, err := h.store.Upload(r.Context(), objectPath, detected, file); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to upload image")
		return "", false
	}

	return objectPath, true
}

func (h *Handler) signArticle(ctx context.Context, b *Berita) {
	h.signImageURL(ctx, b)
	b.Content = h.signContent(ctx, b.Content)
}

func (h *Handler) signImageURL(ctx context.Context, b *Berita) {
	if b.ImageURL == "" {
		return
	}
	signed, err := h.store.PresignGet(ctx, b.ImageURL, 24*time.Hour)
	if err != nil {
		b.ImageURL = ""
		return
	}
	b.ImageURL = signed
}

func (h *Handler) signContent(ctx context.Context, content string) string {
	return resolveImageRefs(content, func(key string) (string, error) {
		return h.store.PresignGet(ctx, key, 24*time.Hour)
	})
}

func (h *Handler) deleteObject(ctx context.Context, objectPath string) {
	if objectPath == "" {
		return
	}
	if strings.Contains(objectPath, "://") {
		if p, err := extractObjectPath(objectPath); err == nil {
			objectPath = p
		}
	}
	_ = h.store.Delete(ctx, objectPath)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrContentRequired), errors.Is(err, ErrContentTooLarge):
		response.Error(w, http.StatusBadRequest, err.Error())
	case err.Error() == "berita not found":
		response.Error(w, http.StatusNotFound, err.Error())
	case err.Error() == "forbidden: not the author":
		response.Error(w, http.StatusForbidden, err.Error())
	default:
		response.Error(w, http.StatusInternalServerError, err.Error())
	}
}

func isValidContentKey(beritaID id.ID, key string) bool {
	if key == "" || strings.ContainsAny(key, "\n\r\t ") {
		return false
	}
	if strings.Contains(key, "://") || strings.HasPrefix(key, "/") || strings.ContainsAny(key, "?") || strings.Contains(key, "..") {
		return false
	}
	prefix := fmt.Sprintf("berita/%s/content/", beritaID)
	return strings.HasPrefix(key, prefix) && len(key) > len(prefix)
}

func extractObjectPath(imageURL string) (string, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(parsed.Path, "/"), nil
}
