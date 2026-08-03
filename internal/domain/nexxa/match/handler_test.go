package match

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match/content"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	svc := NewService(nil)
	return NewHandler(svc, nil)
}

func doPost(t *testing.T, h *Handler, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	switch path {
	case "/api/v1/nexxa/match":
		h.NexxaMatch(rr, req)
	case "/api/v1/nexxa/match/validate-input":
		h.ValidateNexxaInput(rr, req)
	case "/api/v1/nexxa/match/normalize-output":
		h.NormalizeNexxaOutput(rr, req)
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
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
	return env.Success, env.Data, env.Errors
}

func TestValidateNexxaInputHandler(t *testing.T) {
	h := newTestHandler(t)
	path := "/api/v1/nexxa/match/validate-input"
	validBody := `{
		"jawaban_1": "satu", "jawaban_2": "dua", "jawaban_3": "tiga", "jawaban_4": "empat",
		"jawaban_5": "lima", "jawaban_6": "enam", "jawaban_7": "tujuh", "jawaban_8": "delapan"
	}`

	t.Run("success returns 200 and sanitized data", func(t *testing.T) {
		rr := doPost(t, h, path, validBody)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
		}
		ok, data, errs := decodeEnvelope(t, rr)
		if !ok {
			t.Fatalf("expected success true")
		}
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %+v", errs)
		}
		if len(data) != content.NexxaAnswerCount {
			t.Fatalf("expected %d data fields, got %d", content.NexxaAnswerCount, len(data))
		}
		if string(data["jawaban_1"]) != `"satu"` {
			t.Fatalf("unexpected jawaban_1: %s", data["jawaban_1"])
		}
	})

	t.Run("missing field returns 400 with field error", func(t *testing.T) {
		rr := doPost(t, h, path, `{"jawaban_1":"a"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
		ok, _, errs := decodeEnvelope(t, rr)
		if ok {
			t.Fatal("expected success false")
		}
		if len(errs) != 7 {
			t.Fatalf("expected 7 errors, got %d: %+v", len(errs), errs)
		}
		if errs[0].Field != "jawaban_2" {
			t.Fatalf("unexpected first error: %+v", errs[0])
		}
	})

	t.Run("invalid json returns 400", func(t *testing.T) {
		rr := doPost(t, h, path, `{"jawaban_1":`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
		ok, _, _ := decodeEnvelope(t, rr)
		if ok {
			t.Fatal("expected success false")
		}
	})
}

func TestNormalizeNexxaOutputHandler(t *testing.T) {
	h := newTestHandler(t)
	path := "/api/v1/nexxa/match/normalize-output"
	wellFormed := `{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":65,"persentase_akuntansi":20,"persentase_hotel":15}`

	t.Run("success returns 200 with data", func(t *testing.T) {
		body := `{"raw":` + string(mustJSON(t, wellFormed)) + `}`
		rr := doPost(t, h, path, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
		}
		ok, data, errs := decodeEnvelope(t, rr)
		if !ok {
			t.Fatalf("expected success true")
		}
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %+v", errs)
		}
		if string(data["nama_jurusan"]) != `"PPLG"` {
			t.Fatalf("unexpected nama_jurusan: %s", data["nama_jurusan"])
		}
	})

	t.Run("unparseable output returns 422", func(t *testing.T) {
		body := `{"raw":` + string(mustJSON(t, "tidak ada json")) + `}`
		rr := doPost(t, h, path, body)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d (body=%s)", rr.Code, rr.Body.String())
		}
		ok, _, errs := decodeEnvelope(t, rr)
		if ok {
			t.Fatal("expected success false")
		}
		if len(errs) != 1 || errs[0].Message != "Could not parse a valid JSON object from model output." {
			t.Fatalf("unexpected errors: %+v", errs)
		}
	})

	t.Run("empty raw returns 422", func(t *testing.T) {
		body := `{"raw":` + string(mustJSON(t, "   ")) + `}`
		rr := doPost(t, h, path, body)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rr.Code)
		}
		ok, _, _ := decodeEnvelope(t, rr)
		if ok {
			t.Fatal("expected success false")
		}
	})

	t.Run("invalid nama_jurusan returns 422", func(t *testing.T) {
		raw := `{"nama_jurusan":"MIPA","alasan":"x","persentase_pplg":50,"persentase_akuntansi":25,"persentase_hotel":25}`
		body := `{"raw":` + string(mustJSON(t, raw)) + `}`
		rr := doPost(t, h, path, body)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rr.Code)
		}
	})

	t.Run("markdown fenced output returns 200", func(t *testing.T) {
		raw := "```json\n" + wellFormed + "\n```"
		body := `{"raw":` + string(mustJSON(t, raw)) + `}`
		rr := doPost(t, h, path, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
		}
	})
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}