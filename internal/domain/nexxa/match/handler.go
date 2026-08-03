package match

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
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match/content"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/response"
)

type Handler struct {
	svc   *Service
	limit func(http.Handler) http.Handler
}

func NewHandler(svc *Service, limit func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, limit: limit}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/nexxa/match", h.limit(http.HandlerFunc(h.NexxaMatch)))
	mux.Handle("POST /api/v1/nexxa/match/validate-input", http.HandlerFunc(h.ValidateNexxaInput))
	mux.Handle("POST /api/v1/nexxa/match/normalize-output", http.HandlerFunc(h.NormalizeNexxaOutput))
}

func (h *Handler) NexxaMatch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input NexxaRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.NexxaMatch(r.Context(), input.Answers())
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAnswersRequired), errors.Is(err, ErrAnswerTooLong):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, nexxa.ErrN8NTimeout):
		response.Error(w, http.StatusGatewayTimeout, err.Error())
	case errors.Is(err, nexxa.ErrN8NUnavailable):
		response.Error(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, ErrNexxaOutputInvalid):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Handler) ValidateNexxaInput(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		logNexxaInput(r, false, nil, nil)
		response.JSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"errors":  []content.APIError{{Message: "invalid request body"}},
		})
		return
	}

	data, errs := h.svc.ValidateNexxaInput(raw)
	if len(errs) > 0 {
		logNexxaInput(r, false, nil, errs)
		response.JSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"errors":  errs,
		})
		return
	}

	logNexxaInput(r, true, data, nil)
	response.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    data,
	})
}

func (h *Handler) NormalizeNexxaOutput(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input NormalizeOutputRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		logNexxaOutput(r, false, "", nil, nil)
		response.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"success": false,
			"errors":  []content.APIError{{Message: "invalid request body"}},
		})
		return
	}

	data, errs := h.svc.NormalizeNexxaOutput(input.Raw)
	if len(errs) > 0 {
		logNexxaOutput(r, false, input.Raw, nil, errs)
		response.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"success": false,
			"errors":  errs,
		})
		return
	}

	logNexxaOutput(r, true, input.Raw, data, nil)
	response.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    data,
	})
}

func logNexxaInput(r *http.Request, ok bool, data map[string]string, errs []content.APIError) {
	flag := "ok"
	if !ok {
		flag = "failed"
	}
	fields := make([]string, 0, content.NexxaAnswerCount)
	for i := 1; i <= content.NexxaAnswerCount; i++ {
		key := fmt.Sprintf("jawaban_%d", i)
		fields = append(fields, fmt.Sprintf("%s:len=%d,sha=%s", key, len(data[key]), shortSHA(data[key])))
	}
	detail := strings.Join(fields, " ")
	if len(errs) > 0 {
		detail += " errors=" + errorFields(errs)
	}
	log.Printf("nexxa validate-input: %s %s %s", flag, r.URL.Path, detail)
	for i := 1; i <= content.NexxaAnswerCount; i++ {
		key := fmt.Sprintf("jawaban_%d", i)
		if content.HasPromptInjection(data[key]) {
			log.Printf("nexxa validate-input: WARNING suspicious input flagged in %s (sha=%s)", key, shortSHA(data[key]))
		}
	}
}

func logNexxaOutput(r *http.Request, ok bool, raw string, data *content.NormalizeOutputData, errs []content.APIError) {
	flag := "ok"
	if !ok {
		flag = "failed"
	}
	detail := fmt.Sprintf("raw:len=%d,sha=%s", len(raw), shortSHA(raw))
	if data != nil {
		detail += fmt.Sprintf(" major=%s pct=%d/%d/%d", data.NamaJurusan, data.PersentasePPLG, data.PersentaseAkuntansi, data.PersentaseHotel)
	}
	if len(errs) > 0 {
		detail += " errors=" + errorFields(errs)
	}
	log.Printf("nexxa normalize-output: %s %s %s", flag, r.URL.Path, detail)
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