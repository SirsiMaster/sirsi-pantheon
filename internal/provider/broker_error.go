package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Broker errors retain bounded structured evidence, never the request/auth header.
type InferenceError struct {
	Status              int
	Code, Type, Message string
	RetryAfter          time.Duration
	Busy, Permanent     bool
}

func (e *InferenceError) Error() string {
	return fmt.Sprintf("native inference HTTP %d code=%s type=%s: %s", e.Status, e.Code, e.Type, e.Message)
}
func BrokerError(resp *http.Response, bearer string) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("read native inference HTTP %d error: %w", resp.StatusCode, err)
	}
	var body struct {
		Error struct {
			Code          json.RawMessage
			Type, Message string
		}
	}
	_ = json.Unmarshal(data, &body)
	e := &InferenceError{Status: resp.StatusCode, Code: strings.Trim(string(body.Error.Code), "\""), Type: body.Error.Type, Message: body.Error.Message}
	if e.Message == "" {
		e.Message = "broker rejected request (unstructured body omitted)"
	}
	if bearer != "" {
		e.Message = strings.ReplaceAll(e.Message, bearer, "[redacted]")
		e.Code = strings.ReplaceAll(e.Code, bearer, "[redacted]")
		e.Type = strings.ReplaceAll(e.Type, bearer, "[redacted]")
	}
	text := strings.ToLower(e.Code + " " + e.Type + " " + e.Message)
	e.Permanent = resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 413 || resp.StatusCode == 422 || strings.Contains(text, "context_length") || strings.Contains(text, "context window") || strings.Contains(text, "context mismatch") || strings.Contains(text, "context_mismatch") || strings.Contains(text, "context_capacity") || strings.Contains(text, "context_overflow") || strings.Contains(text, "max_context_exceeded") || strings.Contains(text, "max_seq_len_exceeded") || strings.Contains(text, "input_too_long") || (strings.Contains(text, "context") && (strings.Contains(text, "exceed") || strings.Contains(text, "too long")))
	e.Busy = resp.StatusCode == http.StatusServiceUnavailable && !e.Permanent && strings.TrimSpace(resp.Header.Get("Retry-After")) != ""
	if e.Busy {
		e.RetryAfter = time.Second
		if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && n > 0 {
			e.RetryAfter = time.Duration(n) * time.Second
		} else if t, err := http.ParseTime(resp.Header.Get("Retry-After")); err == nil && time.Until(t) > 0 {
			e.RetryAfter = time.Until(t)
		}
		if e.RetryAfter > time.Minute {
			e.RetryAfter = time.Minute
		}
	}
	return e
}

func BrokerBearer(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~/") {
		h, e := os.UserHomeDir()
		if e != nil {
			return "", fmt.Errorf("resolve broker capability home")
		}
		path = h + path[1:]
	}
	f, e := os.Open(path)
	if e != nil {
		return "", fmt.Errorf("configured broker capability unavailable")
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 {
		return "", fmt.Errorf("configured broker capability must be a regular 0600 file")
	}
	b, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(b) > 4096 {
		return "", fmt.Errorf("configured broker capability unreadable or oversized")
	}
	t := strings.TrimSpace(string(b))
	if t == "" || strings.ContainsAny(t, "\r\n \t") {
		return "", fmt.Errorf("configured broker capability invalid")
	}
	return t, nil
}
