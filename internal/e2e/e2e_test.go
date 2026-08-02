//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/auth"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"
)

func doJSON(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func register(t *testing.T, srvURL, email string) (id.ID, string) {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srvURL+"/api/v1/auth/register", "", map[string]string{
		"email":    email,
		"password": "secret-password",
		"name":     "User",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var out struct {
		User  auth.User `json:"user"`
		Token string    `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	resp.Body.Close()
	require.NotEmpty(t, out.Token)
	return out.User.ID, out.Token
}

func promoteToJurnal(t *testing.T, e *env, userID id.ID) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `UPDATE users SET role = 'jurnal' WHERE id = $1`, userID)
	require.NoError(t, err)
}

func uploadImage(t *testing.T, url, token string) *http.Response {
	t.Helper()
	return uploadPNG(t, url, token)
}

func uploadPNG(t *testing.T, target, token string) *http.Response {
	t.Helper()
	png := make([]byte, 600)
	copy(png, "\x89PNG\r\n\x1a\n")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "photo.png")
	require.NoError(t, err)
	_, err = fw.Write(png)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, target, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func assertObjectExists(t *testing.T, e *env, key string) []byte {
	t.Helper()
	obj, err := e.verifyS3.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(key),
	})
	require.NoError(t, err)
	data, err := io.ReadAll(obj.Body)
	require.NoError(t, err)
	obj.Body.Close()
	return data
}

func assertObjectMissing(t *testing.T, e *env, key string) {
	t.Helper()
	_, err := e.verifyS3.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(key),
	})
	var noSuchKey *types.NoSuchKey
	require.ErrorAs(t, err, &noSuchKey)
}

func extractImageRef(t *testing.T, content string) string {
	t.Helper()
	start := strings.Index(content, "](")
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(content[start+2:], ")")
	require.GreaterOrEqual(t, end, 0)
	return content[start+2 : start+2+end]
}

func TestE2E_Register_PersistsUserAndSession(t *testing.T) {
	e := startE2E(t)
	ctx := context.Background()

	resp := doJSON(t, http.MethodPost, e.server.URL+"/api/v1/auth/register", "", map[string]string{
		"email":    "journalist@example.com",
		"password": "secret-password",
		"name":     "Journalist",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out struct {
		User  auth.User `json:"user"`
		Token string    `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	resp.Body.Close()
	require.NotEmpty(t, out.User.ID)
	require.NotEmpty(t, out.Token)
	require.Equal(t, "user", out.User.Role)

	var email, role string
	err := e.pool.QueryRow(ctx, `SELECT email, role FROM users WHERE id = $1`, out.User.ID).Scan(&email, &role)
	require.NoError(t, err)
	require.Equal(t, "journalist@example.com", email)
	require.Equal(t, "user", role)

	var sessions int
	err = e.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE token = $1`, out.Token).Scan(&sessions)
	require.NoError(t, err)
	require.Equal(t, 1, sessions)
}

func TestE2E_LoginLogout(t *testing.T) {
	e := startE2E(t)
	ctx := context.Background()

	_, regToken := register(t, e.server.URL, "journalist@example.com")

	resp := doJSON(t, http.MethodPost, e.server.URL+"/api/v1/auth/login", "", map[string]string{
		"email":    "journalist@example.com",
		"password": "secret-password",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var login struct {
		User  auth.User `json:"user"`
		Token string    `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login))
	resp.Body.Close()
	require.NotEmpty(t, login.Token)
	require.NotEqual(t, regToken, login.Token)

	var sessions int
	err := e.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE token = $1`, login.Token).Scan(&sessions)
	require.NoError(t, err)
	require.Equal(t, 1, sessions)

	resp = doJSON(t, http.MethodPost, e.server.URL+"/api/v1/auth/logout", login.Token, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	err = e.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE token = $1`, login.Token).Scan(&sessions)
	require.NoError(t, err)
	require.Equal(t, 0, sessions)

	resp = doJSON(t, http.MethodGet, e.server.URL+"/api/v1/berita", login.Token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, e.server.URL+"/api/v1/berita", "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestE2E_Login_WrongPassword(t *testing.T) {
	e := startE2E(t)

	register(t, e.server.URL, "journalist@example.com")

	resp := doJSON(t, http.MethodPost, e.server.URL+"/api/v1/auth/login", "", map[string]string{
		"email":    "journalist@example.com",
		"password": "wrong-password",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestE2E_BeritaLifecycle(t *testing.T) {
	e := startE2E(t)
	ctx := context.Background()

	userID, token := register(t, e.server.URL, "journalist@example.com")
	promoteToJurnal(t, e, userID)

	baseURL := e.server.URL + "/api/v1/berita"

	createResp := doJSON(t, http.MethodPost, baseURL, token, map[string]any{
		"title":          "First News",
		"content":        "Body of the first news",
		"is_achievement": true,
	})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created berita.Berita
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	createResp.Body.Close()
	require.NotEmpty(t, created.ID)
	require.Equal(t, userID, created.AuthorID)
	require.True(t, created.IsAchievement)

	var dbTitle string
	var dbAuthor int64
	var dbContent string
	var dbIsAchievement bool
	err := e.pool.QueryRow(ctx, `SELECT title, author_id FROM berita WHERE id = $1`, created.ID).Scan(&dbTitle, &dbAuthor)
	require.NoError(t, err)
	require.Equal(t, "First News", dbTitle)
	require.Equal(t, int64(userID), dbAuthor)
	err = e.pool.QueryRow(ctx, `SELECT is_achievement FROM berita WHERE id = $1`, created.ID).Scan(&dbIsAchievement)
	require.NoError(t, err)
	require.True(t, dbIsAchievement)

	listResp := doJSON(t, http.MethodGet, baseURL, token, nil)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list []berita.Berita
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	listResp.Body.Close()
	require.Len(t, list, 1)
	require.Equal(t, created.ID, list[0].ID)

	getResp := doJSON(t, http.MethodGet, baseURL+"/"+created.ID.String(), token, nil)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var got berita.Berita
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	getResp.Body.Close()
	require.Equal(t, created.ID, got.ID)

	updateResp := doJSON(t, http.MethodPut, baseURL+"/"+created.ID.String(), token, map[string]any{
		"title":          "Updated News",
		"content":        "Updated body",
		"is_achievement": false,
	})
	require.Equal(t, http.StatusOK, updateResp.StatusCode)
	var updated berita.Berita
	require.NoError(t, json.NewDecoder(updateResp.Body).Decode(&updated))
	updateResp.Body.Close()
	require.Equal(t, "Updated News", updated.Title)
	require.False(t, updated.IsAchievement)

	err = e.pool.QueryRow(ctx, `SELECT title, content FROM berita WHERE id = $1`, created.ID).Scan(&dbTitle, &dbContent)
	require.NoError(t, err)
	require.Equal(t, "Updated News", dbTitle)
	require.Equal(t, "Updated body", dbContent)
	png := make([]byte, 600)
	copy(png, "\x89PNG\r\n\x1a\n")

	imgResp := uploadImage(t, baseURL+"/"+created.ID.String()+"/image", token)
	require.Equal(t, http.StatusOK, imgResp.StatusCode)
	var imgOut struct {
		ImageURL string `json:"image_url"`
	}
	require.NoError(t, json.NewDecoder(imgResp.Body).Decode(&imgOut))
	imgResp.Body.Close()
	require.NotEmpty(t, imgOut.ImageURL)

	var rawImagePath string
	err = e.pool.QueryRow(ctx, `SELECT image_url FROM berita WHERE id = $1`, created.ID).Scan(&rawImagePath)
	require.NoError(t, err)
	require.NotEmpty(t, rawImagePath)

	obj, err := e.verifyS3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(rawImagePath),
	})
	require.NoError(t, err)
	stored, err := io.ReadAll(obj.Body)
	require.NoError(t, err)
	obj.Body.Close()
	require.Equal(t, png, stored)

	signedResp, err := http.Get(imgOut.ImageURL)
	require.NoError(t, err)
	signed, err := io.ReadAll(signedResp.Body)
	require.NoError(t, err)
	signedResp.Body.Close()
	require.Equal(t, http.StatusOK, signedResp.StatusCode)
	require.Equal(t, png, signed)

	inlineResp := uploadImage(t, baseURL+"/"+created.ID.String()+"/images", token)
	require.Equal(t, http.StatusOK, inlineResp.StatusCode)
	var inlineOut struct {
		ImageURL string `json:"image_url"`
	}
	require.NoError(t, json.NewDecoder(inlineResp.Body).Decode(&inlineOut))
	inlineResp.Body.Close()
	require.NotEmpty(t, inlineOut.ImageURL)
	require.True(t, strings.HasPrefix(inlineOut.ImageURL, "berita/"+created.ID.String()+"/content/"))
	require.NotContains(t, inlineOut.ImageURL, "://")

	inlineData := assertObjectExists(t, e, inlineOut.ImageURL)
	require.Equal(t, png, inlineData)

	markdownContent := "# Heading\n\nFirst paragraph.\n\n![photo](" + inlineOut.ImageURL + ")\n\nSecond paragraph."
	updateResp = doJSON(t, http.MethodPut, baseURL+"/"+created.ID.String(), token, map[string]string{
		"title":   "Updated News",
		"content": markdownContent,
	})
	require.Equal(t, http.StatusOK, updateResp.StatusCode)
	updateResp.Body.Close()

	err = e.pool.QueryRow(ctx, `SELECT content FROM berita WHERE id = $1`, created.ID).Scan(&dbContent)
	require.NoError(t, err)
	require.Contains(t, dbContent, inlineOut.ImageURL)
	require.NotContains(t, dbContent, "://")

	getResp = doJSON(t, http.MethodGet, baseURL+"/"+created.ID.String(), token, nil)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	getResp.Body.Close()
	signedRef := extractImageRef(t, got.Content)
	require.True(t, strings.HasPrefix(signedRef, "http"))
	require.Contains(t, signedRef, inlineOut.ImageURL)

	inlineSignedResp, err := http.Get(signedRef)
	require.NoError(t, err)
	inlineSigned, err := io.ReadAll(inlineSignedResp.Body)
	require.NoError(t, err)
	inlineSignedResp.Body.Close()
	require.Equal(t, http.StatusOK, inlineSignedResp.StatusCode)
	require.Equal(t, png, inlineSigned)

	delImgResp := doJSON(t, http.MethodDelete, baseURL+"/"+created.ID.String()+"/images?key="+url.QueryEscape(inlineOut.ImageURL), token, nil)
	require.Equal(t, http.StatusNoContent, delImgResp.StatusCode)
	delImgResp.Body.Close()
	assertObjectMissing(t, e, inlineOut.ImageURL)

	keepResp := uploadImage(t, baseURL+"/"+created.ID.String()+"/images", token)
	require.Equal(t, http.StatusOK, keepResp.StatusCode)
	var keepOut struct {
		ImageURL string `json:"image_url"`
	}
	require.NoError(t, json.NewDecoder(keepResp.Body).Decode(&keepOut))
	keepResp.Body.Close()
	assertObjectExists(t, e, keepOut.ImageURL)
	updateResp = doJSON(t, http.MethodPut, baseURL+"/"+created.ID.String(), token, map[string]string{
		"title":   "Updated News",
		"content": "Body with ![inline](" + keepOut.ImageURL + ")",
	})
	require.Equal(t, http.StatusOK, updateResp.StatusCode)
	updateResp.Body.Close()

	deleteResp := doJSON(t, http.MethodDelete, baseURL+"/"+created.ID.String(), token, nil)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	deleteResp.Body.Close()

	var count int
	err = e.pool.QueryRow(ctx, `SELECT COUNT(*) FROM berita WHERE id = $1`, created.ID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	assertObjectMissing(t, e, rawImagePath)
	assertObjectMissing(t, e, keepOut.ImageURL)
}

func TestE2E_RoleAndAuthorization(t *testing.T) {
	e := startE2E(t)

	userID, token := register(t, e.server.URL, "journalist@example.com")
	baseURL := e.server.URL + "/api/v1/berita"

	createResp := doJSON(t, http.MethodPost, baseURL, token, map[string]string{
		"title":   "First News",
		"content": "Body",
	})
	require.Equal(t, http.StatusForbidden, createResp.StatusCode)
	createResp.Body.Close()

	promoteToJurnal(t, e, userID)

	createResp = doJSON(t, http.MethodPost, baseURL, token, map[string]string{
		"title":   "First News",
		"content": "Body",
	})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created berita.Berita
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	createResp.Body.Close()

	otherID, otherToken := register(t, e.server.URL, "other@example.com")
	promoteToJurnal(t, e, otherID)

	updateResp := doJSON(t, http.MethodPut, baseURL+"/"+created.ID.String(), otherToken, map[string]string{
		"title":   "Hijacked",
		"content": "Nope",
	})
	require.Equal(t, http.StatusForbidden, updateResp.StatusCode)
	updateResp.Body.Close()

	deleteResp := doJSON(t, http.MethodDelete, baseURL+"/"+created.ID.String(), otherToken, nil)
	require.Equal(t, http.StatusForbidden, deleteResp.StatusCode)
	deleteResp.Body.Close()

	imgResp := uploadImage(t, baseURL+"/"+created.ID.String()+"/images", otherToken)
	require.Equal(t, http.StatusForbidden, imgResp.StatusCode)
	imgResp.Body.Close()

	delImgResp := doJSON(t, http.MethodDelete, baseURL+"/"+created.ID.String()+"/images?key="+url.QueryEscape("berita/"+created.ID.String()+"/content/abc.png"), otherToken, nil)
	require.Equal(t, http.StatusForbidden, delImgResp.StatusCode)
	delImgResp.Body.Close()

	badKeyResp := doJSON(t, http.MethodDelete, baseURL+"/"+created.ID.String()+"/images?key="+url.QueryEscape("berita/"+created.ID.String()+"/cover.png"), token, nil)
	require.Equal(t, http.StatusBadRequest, badKeyResp.StatusCode)
	badKeyResp.Body.Close()

	missingResp := doJSON(t, http.MethodGet, baseURL+"/999999999999999999", token, nil)
	require.Equal(t, http.StatusNotFound, missingResp.StatusCode)
	missingResp.Body.Close()

	noAuthResp := doJSON(t, http.MethodGet, baseURL, "", nil)
	require.Equal(t, http.StatusOK, noAuthResp.StatusCode)
	noAuthResp.Body.Close()

	badTokenResp := doJSON(t, http.MethodGet, baseURL, "invalid-token", nil)
	require.Equal(t, http.StatusOK, badTokenResp.StatusCode)
	badTokenResp.Body.Close()
}
