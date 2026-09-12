package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

const remoteControlBodyLimit = 8 << 20
const remoteControlActionBodyLimit = 64 << 10

func firstNonEmptyControlEndpoint(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func controlEndpointURL(raw string) (string, error) {
	return controlURL(raw, "/api/control")
}

func controlActionEndpointURL(raw string) (string, error) {
	return controlURL(raw, "/api/control/action")
}

func controlURL(raw, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("control endpoint must be an absolute URL: %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("control endpoint scheme %q is unsupported", parsed.Scheme)
	}
	if parsed.Scheme == "http" && !isLoopbackControlHost(parsed.Hostname()) {
		return "", fmt.Errorf("plain HTTP control endpoints are restricted to loopback hosts; use HTTPS for %q", parsed.Hostname())
	}
	if parsed.Path != "" && parsed.Path != "/" && parsed.Path != "/api/control" {
		return "", fmt.Errorf("control endpoint path must be empty, /, or /api/control, got %q", parsed.Path)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("control endpoint must not include query or fragment")
	}
	parsed.Path = path
	return parsed.String(), nil
}

func isLoopbackControlHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 127
	}
	return ip.Equal(net.ParseIP("::1"))
}

func fetchRemoteControl(ctx context.Context, rawEndpoint, token string) ([]byte, error) {
	return fetchRemoteControlWithRole(ctx, rawEndpoint, token, "", "")
}

func fetchRemoteControlWithRole(ctx context.Context, rawEndpoint, token, expectedRoleID, expectedRoleSHA256 string) ([]byte, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("fetch control snapshot: bearer token is required for remote control")
	}
	endpoint, err := controlEndpointURL(rawEndpoint)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build control request: %w", err)
	}
	if token = strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch control snapshot: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, remoteControlBodyLimit+1))
	if readErr != nil {
		return nil, fmt.Errorf("read control snapshot: %w", readErr)
	}
	if len(body) > remoteControlBodyLimit {
		return nil, fmt.Errorf("control snapshot exceeds %d-byte limit", remoteControlBodyLimit)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("control snapshot returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := validateRemoteControlSnapshotWithRole(body, expectedRoleID, expectedRoleSHA256); err != nil {
		return nil, err
	}
	return body, nil
}

func validateRemoteControlSnapshot(body []byte) error {
	return validateRemoteControlSnapshotWithRole(body, "", "")
}

func validateRemoteControlSnapshotWithRole(body []byte, expectedRoleID, expectedRoleSHA256 string) error {
	if err := routerboard.ValidateJSONNoDuplicateKeys(body); err != nil {
		return fmt.Errorf("control snapshot JSON is ambiguous: %w", err)
	}
	var envelope routerboard.ControlEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("control snapshot is not a valid worker-control envelope: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("control snapshot contains multiple JSON values")
		}
		return fmt.Errorf("control snapshot contains trailing JSON: %w", err)
	}
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("control snapshot envelope: %w", err)
	}
	if err := routerboard.RequireRoleReceiptReference(envelope.RoleReceiptID, envelope.RoleReceiptSHA256, expectedRoleID, expectedRoleSHA256); err != nil {
		return fmt.Errorf("control snapshot role receipt: %w", err)
	}
	return nil
}

func readControlActionRequest(source string) ([]byte, error) {
	var reader io.Reader = os.Stdin
	var file *os.File
	if strings.TrimSpace(source) != "" && source != "-" {
		clean := filepath.Clean(source)
		opened, err := os.Open(clean) //nolint:gosec // explicit operator-selected request file
		if err != nil {
			return nil, fmt.Errorf("open control action request %q: %w", clean, err)
		}
		file = opened
		defer func() { _ = file.Close() }()
		reader = file
	}
	body, err := io.ReadAll(io.LimitReader(reader, remoteControlActionBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read control action request: %w", err)
	}
	if len(body) > remoteControlActionBodyLimit {
		return nil, fmt.Errorf("control action request exceeds %d-byte limit", remoteControlActionBodyLimit)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		if err == nil {
			err = fmt.Errorf("request must be a JSON object")
		}
		return nil, fmt.Errorf("invalid control action request: %w", err)
	}
	if err := routerboard.ValidateJSONNoDuplicateKeys(body); err != nil {
		return nil, err
	}
	return body, nil
}

func sendRemoteControlAction(ctx context.Context, rawEndpoint, token string, body []byte) ([]byte, error) {
	return sendRemoteControlActionWithRole(ctx, rawEndpoint, token, body, "", "")
}

func sendRemoteControlActionWithRole(ctx context.Context, rawEndpoint, token string, body []byte, expectedRoleID, expectedRoleSHA256 string) ([]byte, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("send control action: bearer token is required for remote control")
	}
	endpoint, err := controlActionEndpointURL(rawEndpoint)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build control action request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token = strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send control action: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	result, readErr := io.ReadAll(io.LimitReader(response.Body, remoteControlBodyLimit+1))
	if readErr != nil {
		return nil, fmt.Errorf("read control action response: %w", readErr)
	}
	if len(result) > remoteControlBodyLimit {
		return nil, fmt.Errorf("control action response exceeds %d-byte limit", remoteControlBodyLimit)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if err := validateRemoteControlActionFailure(body, result); err != nil {
			return nil, fmt.Errorf("control action returned HTTP %d with invalid failure receipt: %w", response.StatusCode, err)
		}
		var failure routerboard.ControlActionFailure
		if err := json.Unmarshal(result, &failure); err != nil {
			return nil, fmt.Errorf("control action returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(result)))
		}
		return nil, fmt.Errorf("control action rejected HTTP %d: %s", response.StatusCode, failure.Error)
	}
	if err := validateRemoteControlActionResponseWithRole(body, result, expectedRoleID, expectedRoleSHA256); err != nil {
		return nil, err
	}
	return result, nil
}

func validateRemoteControlActionFailure(requestBody, responseBody []byte) error {
	if err := routerboard.ValidateJSONNoDuplicateKeys(responseBody); err != nil {
		return fmt.Errorf("control action failure JSON is ambiguous: %w", err)
	}
	var request routerboard.ControlActionRequest
	requestDecoder := json.NewDecoder(bytes.NewReader(requestBody))
	requestDecoder.DisallowUnknownFields()
	requestErr := requestDecoder.Decode(&request)
	var failure routerboard.ControlActionFailure
	responseDecoder := json.NewDecoder(bytes.NewReader(responseBody))
	responseDecoder.DisallowUnknownFields()
	if err := responseDecoder.Decode(&failure); err != nil {
		return fmt.Errorf("control action failure is not valid JSON: %w", err)
	}
	var trailing any
	if err := responseDecoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("control action failure contains multiple JSON values")
		}
		return fmt.Errorf("control action failure contains trailing JSON: %w", err)
	}
	if requestErr == nil && strings.TrimSpace(failure.Verb) != strings.TrimSpace(request.Verb) {
		return fmt.Errorf("control action failure verb %q does not match %q", failure.Verb, strings.TrimSpace(request.Verb))
	}
	if err := failure.VerifyControlActionFailure(requestBody); err != nil {
		return err
	}
	return nil
}

func validateRemoteControlActionResponse(requestBody, responseBody []byte) error {
	return validateRemoteControlActionResponseWithRole(requestBody, responseBody, "", "")
}

func validateRemoteControlActionResponseWithRole(requestBody, responseBody []byte, expectedRoleID, expectedRoleSHA256 string) error {
	if err := routerboard.ValidateJSONNoDuplicateKeys(requestBody); err != nil {
		return fmt.Errorf("control action request JSON is ambiguous: %w", err)
	}
	if err := routerboard.ValidateJSONNoDuplicateKeys(responseBody); err != nil {
		return fmt.Errorf("control action response JSON is ambiguous: %w", err)
	}
	var request routerboard.ControlActionRequest
	requestDecoder := json.NewDecoder(bytes.NewReader(requestBody))
	requestDecoder.DisallowUnknownFields()
	if err := requestDecoder.Decode(&request); err != nil {
		return fmt.Errorf("control action request is invalid: %w", err)
	}
	var requestTrailing any
	if err := requestDecoder.Decode(&requestTrailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("control action request contains multiple JSON values")
		}
		return fmt.Errorf("control action request contains trailing JSON: %w", err)
	}
	requestVerb := strings.TrimSpace(request.Verb)
	var response routerboard.ControlActionResponse
	responseDecoder := json.NewDecoder(bytes.NewReader(responseBody))
	responseDecoder.DisallowUnknownFields()
	if err := responseDecoder.Decode(&response); err != nil {
		return fmt.Errorf("control action response is not valid JSON: %w", err)
	}
	var responseTrailing any
	if err := responseDecoder.Decode(&responseTrailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("control action response contains multiple JSON values")
		}
		return fmt.Errorf("control action response contains trailing JSON: %w", err)
	}
	if response.Schema != routerboard.ControlSchema {
		return fmt.Errorf("control action response schema %q is unsupported", response.Schema)
	}
	if response.Authority != "canonical-routerstore" {
		return fmt.Errorf("control action response authority %q is not canonical-routerstore", response.Authority)
	}
	if response.Verb != requestVerb {
		return fmt.Errorf("control action response verb %q does not match %q", response.Verb, requestVerb)
	}
	switch requestVerb {
	case "message", "review_request":
		if strings.TrimSpace(response.ItemID) == "" {
			return fmt.Errorf("control action response omitted item_id")
		}
	case "delegate", "cancel_handback", "result_return":
		if strings.TrimSpace(response.TaskID) == "" {
			return fmt.Errorf("control action response omitted task_id")
		}
		if strings.TrimSpace(request.TaskID) == "" || strings.TrimSpace(response.TaskID) != strings.TrimSpace(request.TaskID) {
			return fmt.Errorf("control action response task_id %q does not match requested task_id %q", response.TaskID, request.TaskID)
		}
	case "claim":
		if response.Lease == nil || strings.TrimSpace(response.Lease.Token) == "" || strings.TrimSpace(response.Lease.TaskID) == "" {
			return fmt.Errorf("control action response omitted lease proof")
		}
		lease := response.Lease
		if strings.TrimSpace(lease.Agent) != strings.TrimSpace(request.Agent) {
			return fmt.Errorf("control action lease agent %q does not match requested agent %q", lease.Agent, request.Agent)
		}
		if strings.TrimSpace(lease.Worker) != strings.TrimSpace(request.Worker) {
			return fmt.Errorf("control action lease worker %q does not match requested worker %q", lease.Worker, request.Worker)
		}
		if strings.TrimSpace(lease.ThreadID) != strings.TrimSpace(request.ThreadID) {
			return fmt.Errorf("control action lease thread_id %q does not match requested thread_id %q", lease.ThreadID, request.ThreadID)
		}
		if requestedTaskID := strings.TrimSpace(request.TaskID); requestedTaskID != "" && strings.TrimSpace(lease.TaskID) != requestedTaskID {
			return fmt.Errorf("control action lease task_id %q does not match requested task_id %q", lease.TaskID, requestedTaskID)
		}
		if strings.TrimSpace(response.TaskID) != strings.TrimSpace(lease.TaskID) || lease.Expires.IsZero() || lease.Attempt <= 0 {
			return fmt.Errorf("control action lease proof is incomplete or detached from task_id")
		}
	case "":
		return fmt.Errorf("control action request omitted verb")
	default:
		return fmt.Errorf("control action response has unsupported verb %q", requestVerb)
	}
	if requestVerb == "result_return" && strings.TrimSpace(response.ResultRef) == "" {
		return fmt.Errorf("control action response omitted result_ref")
	}
	if requestVerb == "result_return" && strings.TrimSpace(response.ResultRef) != strings.TrimSpace(request.ResultRef) {
		return fmt.Errorf("control action response result_ref does not match requested result_ref")
	}
	if err := response.VerifyControlActionResponse(requestBody); err != nil {
		return fmt.Errorf("control action response receipt invalid: %w", err)
	}
	if err := routerboard.RequireRoleReceiptReference(response.RoleReceiptID, response.RoleReceiptSHA256, expectedRoleID, expectedRoleSHA256); err != nil {
		return fmt.Errorf("control action response role receipt: %w", err)
	}
	return nil
}

func expectedControlRoleReference() (string, string, error) {
	id := os.Getenv("SIRSI_CONTROL_ROLE_RECEIPT_ID")
	digest := os.Getenv("SIRSI_CONTROL_ROLE_RECEIPT_SHA256")
	if err := routerboard.ValidateRoleReceiptReference(id, digest); err != nil {
		return "", "", fmt.Errorf("control role receipt expectation: %w", err)
	}
	return id, digest, nil
}

func printControlJSON(body []byte) error {
	var out map[string]json.RawMessage
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("validate control snapshot: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
