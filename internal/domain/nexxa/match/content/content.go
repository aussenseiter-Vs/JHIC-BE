package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	NexxaAnswerCount  = 8
	NexxaAnswerMaxLen = 500
)

type APIError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type NormalizeOutputData struct {
	NamaJurusan         string `json:"nama_jurusan"`
	Alasan              string `json:"alasan"`
	PersentasePPLG      int    `json:"persentase_pplg"`
	PersentaseAkuntansi int    `json:"persentase_akuntansi"`
	PersentaseHotel     int    `json:"persentase_hotel"`
}

var (
	scriptBlockRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleBlockRe  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlTagRe     = regexp.MustCompile(`(?i)<[^>]+>`)
	wsRe          = regexp.MustCompile(`\s+`)
	fenceRe       = regexp.MustCompile("(?i)```[a-z]*")

	injectionRe = regexp.MustCompile(`(?i)(ignore\s+(all\s+)?previous\s+instructions|disregard\s+(all\s+)?previous\s+instructions|system\s*:|you\s+are\s+now|as\s+(an\s+)?(ai|language\s+model)|forget\s+(all\s+)?(previous|prior)\s+instructions|developer\s+mode|jailbreak)`)
)

func sanitizeAnswer(s string) string {
	s = scriptBlockRe.ReplaceAllString(s, " ")
	s = styleBlockRe.ReplaceAllString(s, " ")
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func HasPromptInjection(s string) bool {
	return injectionRe.MatchString(s)
}

func sanitizeForLog(s string) string {
	s = wsRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if len(s) > 100 {
		s = s[:100] + "..."
	}
	return s
}

func ValidateNexxaInput(raw map[string]json.RawMessage) (map[string]string, []APIError) {
	var errs []APIError
	out := make(map[string]string, NexxaAnswerCount)
	for i := 1; i <= NexxaAnswerCount; i++ {
		key := fmt.Sprintf("jawaban_%d", i)
		rv, ok := raw[key]
		if !ok {
			errs = append(errs, APIError{Field: key, Message: "This field is required."})
			continue
		}
		var s string
		if err := json.Unmarshal(rv, &s); err != nil {
			errs = append(errs, APIError{Field: key, Message: "Must be a plain string."})
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			errs = append(errs, APIError{Field: key, Message: "This field is required."})
			continue
		}
		if len(s) > NexxaAnswerMaxLen {
			errs = append(errs, APIError{Field: key, Message: "Must be 500 characters or fewer."})
			continue
		}
		out[key] = sanitizeAnswer(s)
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return out, nil
}

func parseModelOutput(raw string) (map[string]json.RawMessage, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = fenceRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &obj); err == nil {
		return obj, nil
	}

	if start := strings.IndexByte(cleaned, '{'); start >= 0 {
		if end := strings.LastIndexByte(cleaned, '}'); end > start {
			var fallback map[string]json.RawMessage
			if err := json.Unmarshal([]byte(cleaned[start:end+1]), &fallback); err == nil {
				return fallback, nil
			}
		}
	}
	return nil, errors.New("no valid JSON object found in model output")
}

func normalizeMajorName(s string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	switch b.String() {
	case "PPLG":
		return "PPLG", true
	case "AKUNTANSI":
		return "Akuntansi", true
	case "PERHOTELAN":
		return "Perhotelan", true
	}
	return "", false
}

func extractPercent(obj map[string]json.RawMessage, key string) (float64, bool) {
	rv, ok := obj[key]
	if !ok {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(rv, &f); err == nil {
		return f, true
	}
	var s string
	if err := json.Unmarshal(rv, &s); err == nil {
		if p, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return p, true
		}
	}
	return 0, false
}

func NormalizeNexxaOutput(raw string) (*NormalizeOutputData, []APIError) {
	if strings.TrimSpace(raw) == "" {
		return nil, []APIError{{Message: "Empty output from model."}}
	}

	obj, err := parseModelOutput(raw)
	if err != nil {
		return nil, []APIError{{Message: "Could not parse a valid JSON object from model output."}}
	}

	var majorStr string
	if rv, ok := obj["nama_jurusan"]; !ok || json.Unmarshal(rv, &majorStr) != nil {
		return nil, []APIError{{Message: "Missing nama_jurusan."}}
	}
	major, ok := normalizeMajorName(majorStr)
	if !ok {
		return nil, []APIError{{Message: fmt.Sprintf("Invalid nama_jurusan: %s.", sanitizeForLog(majorStr))}}
	}

	var alasan string
	if rv, ok := obj["alasan"]; !ok || json.Unmarshal(rv, &alasan) != nil {
		return nil, []APIError{{Message: "Missing alasan."}}
	}
	alasan = strings.TrimSpace(alasan)
	if alasan == "" {
		return nil, []APIError{{Message: "Missing alasan."}}
	}

	pplg, ok := extractPercent(obj, "persentase_pplg")
	if !ok || pplg < 0 || pplg > 100 {
		return nil, []APIError{{Message: "Invalid persentase_pplg."}}
	}
	akuntansi, ok := extractPercent(obj, "persentase_akuntansi")
	if !ok || akuntansi < 0 || akuntansi > 100 {
		return nil, []APIError{{Message: "Invalid persentase_akuntansi."}}
	}
	hotel, ok := extractPercent(obj, "persentase_hotel")
	if !ok || hotel < 0 || hotel > 100 {
		return nil, []APIError{{Message: "Invalid persentase_hotel."}}
	}

	total := pplg + akuntansi + hotel
	if total <= 0 {
		return nil, []APIError{{Message: "Percentages sum to zero."}}
	}

	p := int(math.Round(pplg * 100 / total))
	a := int(math.Round(akuntansi * 100 / total))
	h := int(math.Round(hotel * 100 / total))

	if diff := 100 - (p + a + h); diff != 0 {
		largest, largestVal := "pplg", p
		if a > largestVal {
			largest, largestVal = "akuntansi", a
		}
		if h > largestVal {
			largest = "hotel"
		}
		switch largest {
		case "pplg":
			p += diff
		case "akuntansi":
			a += diff
		case "hotel":
			h += diff
		}
	}

	return &NormalizeOutputData{
		NamaJurusan:         major,
		Alasan:              alasan,
		PersentasePPLG:      p,
		PersentaseAkuntansi: a,
		PersentaseHotel:     h,
	}, nil
}