//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2E_NexxaChat(t *testing.T) {
	e := startE2E(t)
	url := e.server.URL + "/api/v1/nexxa/chat"

	t.Run("valid message returns n8n output", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{
			"chatInput": "halo",
			"sessionId": "session-1",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			Output string `json:"output"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.Equal(t, "hai dari nexxa", out.Output)
	})

	t.Run("empty message rejected", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"chatInput": "   "})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("overlong message rejected", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{
			"chatInput": strings.Repeat("a", 301),
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("malformed body rejected", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]any{
			"chatInput": "halo",
			"sessionId": []string{"a", "b"},
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close()
	})
}

func TestE2E_NexxaMatch(t *testing.T) {
	e := startE2E(t)
	url := e.server.URL + "/api/v1/nexxa/match"

	valid := map[string]string{
		"jawaban_1": "a", "jawaban_2": "b", "jawaban_3": "c", "jawaban_4": "d",
		"jawaban_5": "e", "jawaban_6": "f", "jawaban_7": "g", "jawaban_8": "h",
	}

	t.Run("valid answers return recommendation", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", valid)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			NamaJurusan         string `json:"nama_jurusan"`
			Alasan              string `json:"alasan"`
			PersentasePPLG      int    `json:"persentase_pplg"`
			PersentaseAkuntansi int    `json:"persentase_akuntansi"`
			PersentaseHotel     int    `json:"persentase_hotel"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.Equal(t, "PPLG", out.NamaJurusan)
		require.Equal(t, "cocok", out.Alasan)
		require.Equal(t, 60, out.PersentasePPLG)
		require.Equal(t, 30, out.PersentaseAkuntansi)
		require.Equal(t, 10, out.PersentaseHotel)
	})

	t.Run("missing answers rejected", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"jawaban_1": "a"})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("blank answer rejected", func(t *testing.T) {
		invalid := make(map[string]string, 8)
		for k, v := range valid {
			invalid[k] = v
		}
		invalid["jawaban_4"] = "  "
		resp := doJSON(t, http.MethodPost, url, "", invalid)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close()
	})
}

func TestE2E_NexxaValidateInput(t *testing.T) {
	e := startE2E(t)
	url := e.server.URL + "/api/v1/nexxa/match/validate-input"

	valid := map[string]string{
		"jawaban_1": "satu", "jawaban_2": "dua", "jawaban_3": "tiga", "jawaban_4": "empat",
		"jawaban_5": "lima", "jawaban_6": "enam", "jawaban_7": "tujuh", "jawaban_8": "delapan",
	}

	t.Run("valid answers return sanitized data", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", valid)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			Success bool              `json:"success"`
			Data    map[string]string `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.True(t, out.Success)
		require.Len(t, out.Data, 8)
		require.Equal(t, "satu", out.Data["jawaban_1"])
	})

	t.Run("html input is sanitized", func(t *testing.T) {
		bad := make(map[string]string, 8)
		for k, v := range valid {
			bad[k] = v
		}
		bad["jawaban_2"] = "<b>Saya</b> <script>alert(1)</script>suka komputer"
		resp := doJSON(t, http.MethodPost, url, "", bad)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			Success bool              `json:"success"`
			Data    map[string]string `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.True(t, out.Success)
		require.Equal(t, "Saya suka komputer", out.Data["jawaban_2"])
	})

	t.Run("missing field rejected", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"jawaban_1": "a"})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var out struct {
			Success bool `json:"success"`
			Errors  []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.False(t, out.Success)
		require.Len(t, out.Errors, 7)
		require.Equal(t, "jawaban_2", out.Errors[0].Field)
		require.Equal(t, "This field is required.", out.Errors[0].Message)
	})

	t.Run("oversized field rejected", func(t *testing.T) {
		big := make(map[string]string, 8)
		for k, v := range valid {
			big[k] = v
		}
		big["jawaban_6"] = strings.Repeat("a", 501)
		resp := doJSON(t, http.MethodPost, url, "", big)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var out struct {
			Success bool `json:"success"`
			Errors  []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.False(t, out.Success)
		require.Len(t, out.Errors, 1)
		require.Equal(t, "jawaban_6", out.Errors[0].Field)
		require.Equal(t, "Must be 500 characters or fewer.", out.Errors[0].Message)
	})

	t.Run("non-string field rejected", func(t *testing.T) {
		bad := make(map[string]any, 8)
		for k, v := range valid {
			bad[k] = v
		}
		bad["jawaban_3"] = []string{"a", "b"}
		resp := doJSON(t, http.MethodPost, url, "", bad)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var out struct {
			Success bool `json:"success"`
			Errors  []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.False(t, out.Success)
		require.Len(t, out.Errors, 1)
		require.Equal(t, "jawaban_3", out.Errors[0].Field)
		require.Equal(t, "Must be a plain string.", out.Errors[0].Message)
	})
}

func TestE2E_NexxaNormalizeOutput(t *testing.T) {
	e := startE2E(t)
	url := e.server.URL + "/api/v1/nexxa/match/normalize-output"

	wellFormed := `{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":65,"persentase_akuntansi":20,"persentase_hotel":15}`

	t.Run("well-formed json returns 200", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"raw": wellFormed})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			Success bool `json:"success"`
			Data    struct {
				NamaJurusan     string `json:"nama_jurusan"`
				PersentasePPLG  int    `json:"persentase_pplg"`
				PersentaseHotel int    `json:"persentase_hotel"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.True(t, out.Success)
		require.Equal(t, "PPLG", out.Data.NamaJurusan)
		require.Equal(t, 65, out.Data.PersentasePPLG)
	})

	t.Run("markdown fenced json returns 200", func(t *testing.T) {
		raw := "```json\n" + wellFormed + "\n```"
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"raw": raw})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			Success bool `json:"success"`
			Data    struct {
				NamaJurusan string `json:"nama_jurusan"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.True(t, out.Success)
		require.Equal(t, "PPLG", out.Data.NamaJurusan)
	})

	t.Run("percentages rescaled to sum 100", func(t *testing.T) {
		raw := `{"nama_jurusan":"PPLG","alasan":"x","persentase_pplg":70,"persentase_akuntansi":20,"persentase_hotel":20}`
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"raw": raw})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			Success bool `json:"success"`
			Data    struct {
				PersentasePPLG      int `json:"persentase_pplg"`
				PersentaseAkuntansi int `json:"persentase_akuntansi"`
				PersentaseHotel     int `json:"persentase_hotel"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.True(t, out.Success)
		sum := out.Data.PersentasePPLG + out.Data.PersentaseAkuntansi + out.Data.PersentaseHotel
		require.Equal(t, 100, sum)
	})

	t.Run("unparseable output returns 422", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"raw": "bukan json"})
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
		var out struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		resp.Body.Close()
		require.False(t, out.Success)
		require.Len(t, out.Errors, 1)
		require.Equal(t, "Could not parse a valid JSON object from model output.", out.Errors[0].Message)
	})

	t.Run("invalid major returns 422", func(t *testing.T) {
		raw := `{"nama_jurusan":"MIPA","alasan":"x","persentase_pplg":50,"persentase_akuntansi":25,"persentase_hotel":25}`
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"raw": raw})
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("empty raw returns 422", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, url, "", map[string]string{"raw": "   "})
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
		resp.Body.Close()
	})
}