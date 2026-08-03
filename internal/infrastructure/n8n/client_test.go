package n8n_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/n8n"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func readBody(t *testing.T, r *http.Request) map[string]string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	var m map[string]string
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func TestClient_Chat(t *testing.T) {
	t.Run("forwards payload and relays response", func(t *testing.T) {
		var gotPath, gotAuth string
		var gotBody map[string]string
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			gotBody = readBody(t, r)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"output": "hai"})
		})

		client := n8n.NewClient(n8n.Config{
			BaseURL:      srv.URL,
			ChatPath:     "/chat",
			ChatUsername: "user",
			ChatPassword: "pass",
			Timeout:      5 * time.Second,
		})
		out, err := client.Chat(context.Background(), "halo", "session-1")
		require.NoError(t, err)
		require.Equal(t, "hai", out.Output)
		require.Equal(t, "/chat", gotPath)
		require.Equal(t, "Basic dXNlcjpwYXNz", gotAuth)
		require.Equal(t, "halo", gotBody["chatInput"])
		require.Equal(t, "session-1", gotBody["sessionId"])
	})

	t.Run("omits basic auth when unset", func(t *testing.T) {
		var gotAuth string
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			json.NewEncoder(w).Encode(map[string]string{"output": "hai"})
		})
		client := n8n.NewClient(n8n.Config{BaseURL: srv.URL, ChatPath: "/chat", Timeout: time.Second})
		_, err := client.Chat(context.Background(), "halo", "session-1")
		require.NoError(t, err)
		require.Empty(t, gotAuth)
	})

	t.Run("unwraps array-of-items response", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"output":"hai"}]`))
		})
		client := n8n.NewClient(n8n.Config{BaseURL: srv.URL, ChatPath: "/chat", Timeout: time.Second})
		out, err := client.Chat(context.Background(), "halo", "session-1")
		require.NoError(t, err)
		require.Equal(t, "hai", out.Output)
	})

	t.Run("non-2xx maps to unavailable", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusForbidden)
		})
		client := n8n.NewClient(n8n.Config{BaseURL: srv.URL, ChatPath: "/chat", Timeout: time.Second})
		_, err := client.Chat(context.Background(), "halo", "session-1")
		require.ErrorIs(t, err, nexxa.ErrN8NUnavailable)
	})

	t.Run("timeout maps to timeout error", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(500 * time.Millisecond)
			json.NewEncoder(w).Encode(map[string]string{"output": "late"})
		})
		client := n8n.NewClient(n8n.Config{BaseURL: srv.URL, ChatPath: "/chat", Timeout: 50 * time.Millisecond})
		_, err := client.Chat(context.Background(), "halo", "session-1")
		require.ErrorIs(t, err, nexxa.ErrN8NTimeout)
	})
}

func TestClient_NexxaMatch(t *testing.T) {
	t.Run("forwards eight answers with secret header", func(t *testing.T) {
		var gotPath, gotSecret string
		var gotBody map[string]string
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotSecret = r.Header.Get("X-JHIC-Secret")
			gotBody = readBody(t, r)
			w.Write([]byte(`{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":60,"persentase_akuntansi":30,"persentase_hotel":10}`))
		})

		client := n8n.NewClient(n8n.Config{
			BaseURL:     srv.URL,
			NexxaPath:   "/nexxa",
			NexxaSecret: "sekret",
			Timeout:     time.Second,
		})
		answers := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
		out, err := client.NexxaMatch(context.Background(), answers)
		require.NoError(t, err)
		require.JSONEq(t, `{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":60,"persentase_akuntansi":30,"persentase_hotel":10}`, out)
		require.Equal(t, "/nexxa", gotPath)
		require.Equal(t, "sekret", gotSecret)
		for i := 0; i < 8; i++ {
			require.Equal(t, answers[i], gotBody[fmt.Sprintf("jawaban_%d", i+1)])
		}
	})

	t.Run("relays raw output text", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"nama_jurusan":"PPLG","alasan":"cocok"}`))
		})
		client := n8n.NewClient(n8n.Config{BaseURL: srv.URL, NexxaPath: "/nexxa", Timeout: time.Second})
		out, err := client.NexxaMatch(context.Background(), []string{"a", "b", "c", "d", "e", "f", "g", "h"})
		require.NoError(t, err)
		require.Equal(t, `{"nama_jurusan":"PPLG","alasan":"cocok"}`, out)
	})

	t.Run("non-2xx maps to unavailable", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		})
		client := n8n.NewClient(n8n.Config{BaseURL: srv.URL, NexxaPath: "/nexxa", Timeout: time.Second})
		_, err := client.NexxaMatch(context.Background(), []string{"a", "b", "c", "d", "e", "f", "g", "h"})
		require.ErrorIs(t, err, nexxa.ErrN8NUnavailable)
	})
}
