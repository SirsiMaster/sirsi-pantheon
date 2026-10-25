package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Each read owns an immutable filter/cursor tuple and generation. Cancelling a
// read invalidates late success AND late failure; it never rewinds newer data.
func (s *activityScreen) loadMaat(cursor string) tea.Cmd {
	if s.maatCancel != nil {
		s.maatCancel()
	}
	s.maatGeneration++
	generation := s.maatGeneration
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	s.maatCancel = cancel
	scope := s.maatScope
	if scope == "" {
		scope = "all"
	}
	args := []string{"casebook", "--limit", "50", "--scope", scope}
	if s.maatStatus != "" {
		args = append(args, "--status", s.maatStatus)
	}
	if cursor != "" {
		args = append(args, "--cursor", cursor)
	}
	if s.maatQuery != "" {
		args = append(args, "--", s.maatQuery)
	}
	snapshot, queryID := s.maatReport.History.Snapshot, s.maatReport.History.QueryID
	if cursor == "" {
		s.maatReport = maatCasebookReport{}
	}
	s.maatCursor = cursor
	s.maatState = stateLoading
	s.maatErr = nil
	runner := getRunner()
	return func() tea.Msg {
		defer cancel()
		report, err := readMaatReport(ctx, runner, args)
		return maatCasebookLoaded{generation: generation, cursor: cursor, snapshot: snapshot, queryID: queryID, report: report, err: err}
	}
}

func readMaatReport(ctx context.Context, runner Runner, args []string) (maatCasebookReport, error) {
	var report maatCasebookReport
	if err := ctx.Err(); err != nil {
		return report, err
	}
	raw, err := runner.RunJSON(ctx, "maat", args...)
	if err != nil {
		return report, err
	}
	if err = ctx.Err(); err != nil {
		return report, err
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil || envelope["cases"] == nil || string(envelope["cases"]) == "null" || envelope["history"] == nil {
		return report, errors.New("canonical history projection unavailable")
	}
	if err = json.Unmarshal(raw, &report); err != nil {
		return report, errors.New("invalid canonical history projection")
	}
	switch report.History.Coverage {
	case "complete", "partial", "unavailable":
	default:
		return report, errors.New("history coverage unavailable")
	}
	if report.History.Coverage != "complete" && len(report.Cases) != 0 {
		return report, errors.New("provisional history cases refused")
	}
	return report, nil
}

func (s *activityScreen) CancelRead() {
	s.maatGeneration++
	if s.maatCancel != nil {
		s.maatCancel()
		s.maatCancel = nil
	}
	if s.maatState == stateLoading {
		s.maatState = stateIdle
	}
}
func (s *activityScreen) ResumeRead() tea.Cmd {
	if s.showMaat && s.maatState == stateIdle {
		return s.loadMaat("")
	}
	return nil
}
func (s *activityScreen) HandleTextKey(key string) (bool, tea.Cmd) {
	if !s.showMaat || !s.maatEditing {
		return false, nil
	}
	switch key {
	case "esc":
		s.maatEditing = false
		return true, s.ResumeRead()
	case "enter":
		s.maatEditing = false
		s.maatQuery = s.maatDraft
		return true, s.loadMaat("")
	case "backspace":
		if s.maatDraft != "" {
			_, n := utf8.DecodeLastRuneInString(s.maatDraft)
			s.maatDraft = s.maatDraft[:len(s.maatDraft)-n]
		}
	case "space":
		if len(s.maatDraft) < 512 {
			s.maatDraft += " "
		}
	default:
		if utf8.RuneCountInString(key) == 1 && !strings.ContainsAny(key, "\x00\r\n") && len(s.maatDraft)+len(key) <= 512 {
			s.maatDraft += key
		}
	}
	return true, nil
}
func (s *activityScreen) maatHistoryLabel() string {
	h := s.maatReport.History
	count := fmt.Sprintf("%d page cases; %d candidate matches (%s)", h.Returned, h.CandidateMatches, h.CountScope)
	if h.TotalComplete && h.Total != nil {
		count += fmt.Sprintf("; query total %d", *h.Total)
	}
	if h.NextCursor != "" {
		count += "; n load older/continue"
	}
	if s.maatReport.Integrity.InvalidCount > 0 {
		count += fmt.Sprintf("; %d invalid records: no absence proof", s.maatReport.Integrity.InvalidCount)
	}
	return count
}

func (s *activityScreen) maatQueryLabel() string {
	scope := s.maatScope
	if scope == "" {
		scope = "all"
	}
	status := s.maatStatus
	if status == "" {
		status = "all"
	}
	return fmt.Sprintf("scope=%s status=%s query=%q", scope, status, s.maatQuery)
}
