// Package oplog provides operation logging for destructive actions.
//
// Every file deletion, cleanup, or purge is logged to
// ~/Library/Logs/sirsi/operations.log with timestamp, action, path,
// and bytes affected. Disable with SIRSI_NO_OPLOG=1.
package oplog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	logFile *os.File
)

// Path returns the operations-log location. The writer (Log) and the reader
// (ReadLast) both resolve through here so they can never disagree.
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "sirsi", "operations.log")
}

// Log records a destructive operation.
func Log(action, path string, bytes int64) {
	if os.Getenv("SIRSI_NO_OPLOG") == "1" {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		logPath := Path()
		_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		logFile = f
	}

	ts := time.Now().Format("2006-01-02T15:04:05")
	sizeStr := ""
	if bytes > 0 {
		sizeStr = fmt.Sprintf(" (%d bytes)", bytes)
	}
	_, _ = fmt.Fprintf(logFile, "%s  %s  %s%s\n", ts, action, path, sizeStr)
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}

// Entry is one parsed operations-log line — the structured form of the ledger.
// This is THE single reader of the free-text log format Log writes: every
// surface (CLI `sirsi activity --json`, TUI, menubar, dashboard) consumes
// entries through here, so the line format never becomes a private contract
// parsed independently elsewhere (TUI design proof, gap V4).
type Entry struct {
	// Time is the timestamp exactly as written ("2006-01-02T15:04:05", local
	// clock, no zone recorded) — surfaced verbatim rather than re-interpreted
	// into a zone the log never captured.
	Time   string `json:"time"`
	Action string `json:"action"` // e.g. "clean", "purge"
	Target string `json:"target"` // the path acted on
	Bytes  int64  `json:"bytes"`  // bytes affected; 0 when the line has no size suffix
	Source string `json:"source"` // provenance of this entry; always "oplog" today
}

// ParseLine parses one "ts  action  path (N bytes)" line written by Log.
// The " (N bytes)" suffix is optional (Log omits it for zero bytes). Returns
// ok=false for blank or malformed lines, which callers skip.
func ParseLine(line string) (Entry, bool) {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return Entry{}, false
	}
	// Log's field separator is exactly two spaces; the target path may itself
	// contain spaces, so split only twice.
	parts := strings.SplitN(line, "  ", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Entry{}, false
	}
	e := Entry{Time: parts[0], Action: parts[1], Target: parts[2], Source: "oplog"}
	// Strip the optional trailing " (N bytes)".
	if strings.HasSuffix(e.Target, " bytes)") {
		if i := strings.LastIndex(e.Target, " ("); i >= 0 {
			numStr := strings.TrimSuffix(e.Target[i+2:], " bytes)")
			if n, err := strconv.ParseInt(numStr, 10, 64); err == nil && n >= 0 {
				e.Bytes = n
				e.Target = e.Target[:i]
			}
		}
	}
	return e, true
}

// ReadLast returns up to n parsed entries from the log at path, NEWEST FIRST.
// A missing log file is not an error — it returns an empty slice (nothing has
// been cleaned yet is a normal state). n <= 0 means all entries.
func ReadLast(path string, n int) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // paths can be long
	for sc.Scan() {
		if e, ok := ParseLine(sc.Text()); ok {
			entries = append(entries, e)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// Newest first: the file is append-ordered, so reverse.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}
