package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/llm"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.NewDecoder(r.Body).Decode(&m))
	return m
}

func TestClient_Complete(t *testing.T) {
	t.Run("posts messages and returns content", func(t *testing.T) {
		var gotAuth string
		var gotModel string
		var gotMsgs []map[string]string
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v1/chat/completions", r.URL.Path)
			gotAuth = r.Header.Get("Authorization")
			m := decodeBody(t, r)
			gotModel, _ = m["model"].(string)
			raw, _ := json.Marshal(m["messages"])
			require.NoError(t, json.Unmarshal(raw, &gotMsgs))
			w.Write([]byte(`{"choices":[{"message":{"content":"hai"}}]}`))
		})

		client := llm.NewClient(llm.Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "model-x", Timeout: time.Second})
		out, err := client.Complete(context.Background(), []nexxa.Message{{Role: "system", Content: "s"}, {Role: "user", Content: "halo"}}, false)
		require.NoError(t, err)
		require.Equal(t, "hai", out)
		require.Equal(t, "Bearer k", gotAuth)
		require.Equal(t, "model-x", gotModel)
		require.Len(t, gotMsgs, 2)
		require.Equal(t, "user", gotMsgs[1]["role"])
		require.Equal(t, "halo", gotMsgs[1]["content"])
	})

	t.Run("json mode sets response_format", func(t *testing.T) {
		var gotJSON any
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			m := decodeBody(t, r)
			gotJSON = m["response_format"]
			w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
		})
		client := llm.NewClient(llm.Config{BaseURL: srv.URL, Model: "m", Timeout: time.Second})
		_, err := client.Complete(context.Background(), []nexxa.Message{{Role: "user", Content: "x"}}, true)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"type": "json_object"}, gotJSON)
	})

	t.Run("omits auth when key empty", func(t *testing.T) {
		var gotAuth string
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.Write([]byte(`{"choices":[{"message":{"content":"hai"}}]}`))
		})
		client := llm.NewClient(llm.Config{BaseURL: srv.URL, Model: "m", Timeout: time.Second})
		_, err := client.Complete(context.Background(), []nexxa.Message{{Role: "user", Content: "x"}}, false)
		require.NoError(t, err)
		require.Empty(t, gotAuth)
	})

	t.Run("empty choices rejected", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"choices":[]}`))
		})
		client := llm.NewClient(llm.Config{BaseURL: srv.URL, Model: "m", Timeout: time.Second})
		_, err := client.Complete(context.Background(), []nexxa.Message{{Role: "user", Content: "x"}}, false)
		require.ErrorIs(t, err, nexxa.ErrUpstreamUnavailable)
	})

	t.Run("non-2xx maps to unavailable", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		})
		client := llm.NewClient(llm.Config{BaseURL: srv.URL, Model: "m", Timeout: time.Second})
		_, err := client.Complete(context.Background(), []nexxa.Message{{Role: "user", Content: "x"}}, false)
		require.ErrorIs(t, err, nexxa.ErrUpstreamUnavailable)
	})

	t.Run("timeout maps to timeout error", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
		})
		client := llm.NewClient(llm.Config{BaseURL: srv.URL, Model: "m", Timeout: 50 * time.Millisecond})
		_, err := client.Complete(context.Background(), []nexxa.Message{{Role: "user", Content: "x"}}, false)
		require.ErrorIs(t, err, nexxa.ErrUpstreamTimeout)
	})
}

func TestClient_Embed(t *testing.T) {
	t.Run("returns embeddings for each input", func(t *testing.T) {
		var gotModel any
		var gotInputs []string
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v1/embeddings", r.URL.Path)
			m := decodeBody(t, r)
			gotModel = m["model"]
			raw, _ := json.Marshal(m["input"])
			require.NoError(t, json.Unmarshal(raw, &gotInputs))
			w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`))
		})

		client := llm.NewClient(llm.Config{BaseURL: srv.URL, EmbedURL: srv.URL + "/v1", EmbedModel: "emb", Timeout: time.Second})
		vecs, err := client.Embed(context.Background(), []string{"a", "b"})
		require.NoError(t, err)
		require.Len(t, vecs, 2)
		require.Equal(t, []float32{0.1, 0.2}, vecs[0])
		require.Equal(t, []float32{0.3, 0.4}, vecs[1])
		require.Equal(t, "emb", gotModel)
		require.Equal(t, []string{"a", "b"}, gotInputs)
	})

	t.Run("empty inputs return empty result", func(t *testing.T) {
		client := llm.NewClient(llm.Config{BaseURL: "http://example.invalid", EmbedModel: "emb", Timeout: time.Second})
		vecs, err := client.Embed(context.Background(), nil)
		require.NoError(t, err)
		require.Empty(t, vecs)
	})

	t.Run("empty embedding rejected", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":[{"embedding":[]}]}`))
		})
		client := llm.NewClient(llm.Config{BaseURL: srv.URL, EmbedModel: "emb", Timeout: time.Second})
		_, err := client.Embed(context.Background(), []string{"a"})
		require.ErrorIs(t, err, nexxa.ErrUpstreamUnavailable)
	})
}