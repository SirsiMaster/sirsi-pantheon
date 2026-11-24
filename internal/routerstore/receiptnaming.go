package routerstore

// receiptnaming.go — a merged PR may close every task its description names
// (owner decision 2026-11-24). A pr receipt already recorded for another task
// is accepted again only when the service itself fetches the PR from GitHub and
// finds, all of:
//
//	merged, with a merge time
//	body never edited after the merge (lastEditedAt absent or before mergedAt)
//	body names this exact task id in an explicit "Task `id`" / "Closes task id" form
//
// Anyone with write access can edit a PR body after merge; refusing a body
// edited at or after the merge keeps a later edit from naming unrelated tasks.
// Anything unavailable refuses (fail closed). ci:/tag:/maat: receipts stay
// single-use, and a first use of any receipt never reaches this file.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// receiptUsedError is the "already used" refusal. It is ErrReceiptInvalid to
// callers and carries the text lane agents have always seen.
type receiptUsedError struct{ msg string }

func (e *receiptUsedError) Error() string        { return e.msg }
func (e *receiptUsedError) Is(target error) bool { return target == ErrReceiptInvalid }

// PRNamingVerifier is the optional second capability of a ReceiptVerifier: it
// proves a merged PR's description names a task. A verifier without it never
// permits reuse.
type PRNamingVerifier interface {
	PRNamesTask(ctx context.Context, value, taskID string) error
}

func prNamesTask(value, taskID string) error {
	v, ok := getReceiptVerifier().(PRNamingVerifier)
	if !ok {
		return errors.New("verifier cannot read PR descriptions")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return v.PRNamesTask(ctx, value, taskID)
}

const prNamingQuery = `query($o:String!,$r:String!,$n:Int!){repository(owner:$o,name:$r){pullRequest(number:$n){merged mergedAt lastEditedAt body}}}`

func (g *githubVerifier) PRNamesTask(ctx context.Context, value, taskID string) error {
	if g.token == "" {
		return errors.New("router has no GitHub token")
	}
	m := rePR.FindStringSubmatch(value)
	if m == nil {
		return errors.New("not a pr receipt")
	}
	owner, repo, _ := strings.Cut(m[1], "/")
	n, _ := strconv.Atoi(m[2])
	payload, _ := json.Marshal(map[string]any{"query": prNamingQuery, "variables": map[string]any{"o": owner, "r": repo, "n": n}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("PR metadata unavailable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PR metadata unavailable: GitHub GraphQL answered %d", resp.StatusCode)
	}
	var out struct {
		Errors []json.RawMessage `json:"errors"`
		Data   struct {
			Repository *struct {
				PullRequest *struct {
					Merged       bool    `json:"merged"`
					MergedAt     *string `json:"mergedAt"`
					LastEditedAt *string `json:"lastEditedAt"`
					Body         string  `json:"body"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("PR metadata unavailable: %v", err)
	}
	if len(out.Errors) > 0 || out.Data.Repository == nil || out.Data.Repository.PullRequest == nil {
		return errors.New("PR metadata unavailable")
	}
	pr := out.Data.Repository.PullRequest
	if !pr.Merged || pr.MergedAt == nil {
		return errors.New("PR is not merged")
	}
	merged, err := time.Parse(time.RFC3339, *pr.MergedAt)
	if err != nil {
		return errors.New("PR merge time unavailable")
	}
	if pr.LastEditedAt != nil {
		edited, err := time.Parse(time.RFC3339, *pr.LastEditedAt)
		if err != nil {
			return errors.New("PR edit time unavailable")
		}
		if !edited.Before(merged) {
			return errors.New("PR description was edited at or after merge")
		}
	}
	if !bodyNamesTask(pr.Body, taskID) {
		return fmt.Errorf("PR description does not name task %s", taskID)
	}
	return nil
}

// idByte: characters a task id may contain.
func idByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.'
}

// bodyNamesTask reports whether body has `task <id>` where <id> is a whole
// token: it follows the word "task" (any case; so "Task `id`", "Closes task id",
// "Task: id"), optionally in backticks, and neither side continues the id. A
// trailing "." that ends a sentence is not part of the id.
func bodyNamesTask(body, id string) bool {
	if id == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(body[from:], id)
		if i < 0 {
			return false
		}
		start := from + i
		from = start + 1
		end := start + len(id)
		// What precedes: [task][optional colon/space][optional backtick]id
		p := start
		tick := p > 0 && body[p-1] == '`'
		if tick {
			p--
		}
		for p > 0 && (body[p-1] == ' ' || body[p-1] == '\t' || body[p-1] == ':') {
			p--
		}
		if p < 4 || !strings.EqualFold(body[p-4:p], "task") || (p > 4 && isAlnum(body[p-5])) {
			continue
		}
		if tick {
			if end < len(body) && body[end] == '`' {
				return true
			}
			continue
		}
		if start > 0 && idByte(body[start-1]) {
			continue
		}
		if end < len(body) && idByte(body[end]) && !(body[end] == '.' && (end+1 == len(body) || !idByte(body[end+1]))) {
			continue
		}
		return true
	}
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
