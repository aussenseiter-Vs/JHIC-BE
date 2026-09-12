package cvreview

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/cvreview/content"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestHandler(t *testing.T, client nexxa.AIClient) *Handler {
	t.Helper()
	svc := NewService(client)
	return NewHandler(svc, nil, nil)
}

func doPost(t *testing.T, h *Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	switch path {
	case "/api/v1/nexxa/cv-review":
		h.CvReview(rr, req)
	case "/api/v1/nexxa/cv-review/validate-input":
		h.ValidateCvInput(rr, req)
	case "/api/v1/nexxa/cv-review/normalize-output":
		h.NormalizeCvOutput(rr, req)
	default:
		t.Fatalf("unknown path %q", path)
	}
	return rr
}

func decodeEnvelope(t *testing.T, rr *httptest.ResponseRecorder) (success bool, data map[string]json.RawMessage, errs []content.APIError) {
	t.Helper()
	var env struct {
		Success bool                       `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
		Errors  []content.APIError         `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env), "body=%s", rr.Body.String())
	return env.Success, env.Data, env.Errors
}

const wellFormedCvOutput = `{
	"audit_summary": {
		"score": 80, "tier_label": "Kandidat Kuat", "grade_label": "B+", "summary_text": "Ringkasan.",
		"key_strengths": [], "key_improvements": []
	},
	"metrics": {"format_score": 85, "ats_status": "good"},
	"grammar_issues": [], "recommendations": [], "strengths_detail": []
}`

func TestCvReviewHandler(t *testing.T) {
	t.Run("success returns 200 with normalized data", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.Anything, true).Return(wellFormedCvOutput, nil)
		h := newTestHandler(t, client)

		rr := doPost(t, h, "/api/v1/nexxa/cv-review", `{"cv_text":"CV saya","word_count":3,"page_count":1}`)
		require.Equal(t, http.StatusOK, rr.Code)
		var out struct {
			AuditSummary content.AuditSummary `json:"audit_summary"`
			Metrics      content.Metrics      `json:"metrics"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
		require.Equal(t, 80, out.AuditSummary.Score)
		require.Equal(t, "good", out.Metrics.ATSStatus)
	})

	t.Run("empty cv_text returns 400", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		h := newTestHandler(t, client)
		rr := doPost(t, h, "/api/v1/nexxa/cv-review", `{"cv_text":"  "}`)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("upstream unavailable returns 502", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.Anything, true).Return("", nexxa.ErrUpstreamUnavailable)
		h := newTestHandler(t, client)
		rr := doPost(t, h, "/api/v1/nexxa/cv-review", `{"cv_text":"cv"}`)
		require.Equal(t, http.StatusBadGateway, rr.Code)
	})

	t.Run("invalid ai output returns 422", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.Anything, true).Return("bukan json", nil)
		h := newTestHandler(t, client)
		rr := doPost(t, h, "/api/v1/nexxa/cv-review", `{"cv_text":"cv"}`)
		require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		h := newTestHandler(t, client)
		rr := doPost(t, h, "/api/v1/nexxa/cv-review", `{"cv_text":`)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestValidateCvInputHandler(t *testing.T) {
	h := newTestHandler(t, nil)
	path := "/api/v1/nexxa/cv-review/validate-input"

	t.Run("success returns sanitized data", func(t *testing.T) {
		rr := doPost(t, h, path, `{"cv_text":"<b>Nama</b> Saya","word_count":2}`)
		require.Equal(t, http.StatusOK, rr.Code)
		ok, data, errs := decodeEnvelope(t, rr)
		require.True(t, ok)
		require.Empty(t, errs)
		require.Equal(t, `"Nama Saya"`, string(data["cv_text"]))
	})

	t.Run("missing cv_text returns 400", func(t *testing.T) {
		rr := doPost(t, h, path, `{"word_count":1}`)
		require.Equal(t, http.StatusBadRequest, rr.Code)
		ok, _, errs := decodeEnvelope(t, rr)
		require.False(t, ok)
		require.Len(t, errs, 1)
		require.Equal(t, "cv_text", errs[0].Field)
	})

	t.Run("invalid json returns 400", func(t *testing.T) {
		rr := doPost(t, h, path, `{"cv_text":`)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestNormalizeCvOutputHandler(t *testing.T) {
	h := newTestHandler(t, nil)
	path := "/api/v1/nexxa/cv-review/normalize-output"

	t.Run("well-formed output returns 200", func(t *testing.T) {
		rr := doPost(t, h, path, `{"raw":`+mustJSON(t, wellFormedCvOutput)+`}`)
		require.Equal(t, http.StatusOK, rr.Code)
		ok, data, errs := decodeEnvelope(t, rr)
		require.True(t, ok)
		require.Empty(t, errs)
		var summary content.AuditSummary
		require.NoError(t, json.Unmarshal(data["audit_summary"], &summary))
		require.Equal(t, 80, summary.Score)
	})

	t.Run("unparseable output returns 422", func(t *testing.T) {
		rr := doPost(t, h, path, `{"raw":`+mustJSON(t, "tidak ada json")+`}`)
		require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
		ok, _, errs := decodeEnvelope(t, rr)
		require.False(t, ok)
		require.Len(t, errs, 1)
		require.Equal(t, "Could not parse a valid JSON object from model output.", errs[0].Message)
	})
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}