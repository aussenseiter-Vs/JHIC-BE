package cvreview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/cvreview/content"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/response"
)

type Handler struct {
	svc    *Service
	authMw func(http.Handler) http.Handler
	limit  func(http.Handler) http.Handler
}

func NewHandler(svc *Service, authMw func(http.Handler) http.Handler, limit func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, authMw: authMw, limit: limit}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/nexxa/cv-review", h.authMw(h.limit(http.HandlerFunc(h.CvReview))))
	mux.Handle("POST /api/v1/nexxa/cv-review/validate-input", http.HandlerFunc(h.ValidateCvInput))
	mux.Handle("POST /api/v1/nexxa/cv-review/normalize-output", http.HandlerFunc(h.NormalizeCvOutput))
}

func (h *Handler) CvReview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input CvReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.CvReview(r.Context(), input)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCvTextRequired), errors.Is(err, ErrCvTextTooLong), errors.Is(err, ErrInvalidCounts):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, nexxa.ErrUpstreamTimeout):
		response.Error(w, http.StatusGatewayTimeout, err.Error())
	case errors.Is(err, nexxa.ErrUpstreamUnavailable):
		response.Error(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, ErrCvOutputInvalid):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Handler) ValidateCvInput(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		logCvInput(r, false, nil, nil)
		response.JSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"errors":  []content.APIError{{Message: "invalid request body"}},
		})
		return
	}

	data, errs := h.svc.ValidateCvInput(raw)
	if len(errs) > 0 {
		logCvInput(r, false, nil, errs)
		response.JSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"errors":  errs,
		})
		return
	}

	logCvInput(r, true, data, nil)
	response.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    data,
	})
}

func (h *Handler) NormalizeCvOutput(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input NormalizeOutputRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		logCvOutput(r, false, "", nil, nil)
		response.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"success": false,
			"errors":  []content.APIError{{Message: "invalid request body"}},
		})
		return
	}

	data, errs := h.svc.NormalizeCvOutput(input.Raw)
	if len(errs) > 0 {
		logCvOutput(r, false, input.Raw, nil, errs)
		response.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"success": false,
			"errors":  errs,
		})
		return
	}

	logCvOutput(r, true, input.Raw, data, nil)
	response.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    data,
	})
}

func logCvInput(r *http.Request, ok bool, data map[string]any, errs []content.APIError) {
	flag := "ok"
	if !ok {
		flag = "failed"
	}
	detail := ""
	if cv, ok := data["cv_text"].(string); ok {
		detail = fmt.Sprintf("cv_text:len=%d,sha=%s", len(cv), shortSHA(cv))
	}
	if len(errs) > 0 {
		detail += " errors=" + errorFields(errs)
	}
	log.Printf("cv-review validate-input: %s %s %s", flag, r.URL.Path, detail)
	if cv, ok := data["cv_text"].(string); ok && content.HasPromptInjection(cv) {
		log.Printf("cv-review validate-input: WARNING suspicious input flagged (sha=%s)", shortSHA(cv))
	}
}

func logCvOutput(r *http.Request, ok bool, raw string, data *content.NormalizeOutputData, errs []content.APIError) {
	flag := "ok"
	if !ok {
		flag = "failed"
	}
	detail := fmt.Sprintf("raw:len=%d,sha=%s", len(raw), shortSHA(raw))
	if data != nil {
		detail += fmt.Sprintf(" score=%d ats=%s recs=%d grammar=%d",
			data.AuditSummary.Score, data.Metrics.ATSStatus, len(data.Recommendations), len(data.GrammarIssues))
	}
	if len(errs) > 0 {
		detail += " errors=" + errorFields(errs)
	}
	log.Printf("cv-review normalize-output: %s %s %s", flag, r.URL.Path, detail)
}

func errorFields(errs []content.APIError) string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		if e.Field != "" {
			out = append(out, fmt.Sprintf("%s:%s", e.Field, e.Message))
		} else {
			out = append(out, e.Message)
		}
	}
	return strings.Join(out, ",")
}

func shortSHA(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:4])
}