package chat

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/analytics"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/response"
)

type Handler struct {
	svc       *Service
	limit     func(http.Handler) http.Handler
	analytics *analytics.Service
}

func NewHandler(svc *Service, limit func(http.Handler) http.Handler, tracking ...*analytics.Service) *Handler {
	var a *analytics.Service
	if len(tracking) > 0 {
		a = tracking[0]
	}
	return &Handler{svc: svc, limit: limit, analytics: a}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/nexxa/chat", h.limit(http.HandlerFunc(h.Chat)))
}

func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.Chat(r.Context(), input.ChatInput, input.SessionID)
	if h.analytics != nil {
		props := map[string]any{"message_length": len(input.ChatInput), "success": err == nil}
		if len(input.Topic) <= 80 {
			props["topic"] = input.Topic
		}
		h.analytics.Record(r.Context(), "chat.request", input.SessionID, nil, props)
	}
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrChatMessageRequired), errors.Is(err, ErrChatMessageTooLong):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, nexxa.ErrUpstreamTimeout):
		response.Error(w, http.StatusGatewayTimeout, err.Error())
	case errors.Is(err, nexxa.ErrUpstreamUnavailable):
		response.Error(w, http.StatusBadGateway, err.Error())
	default:
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
