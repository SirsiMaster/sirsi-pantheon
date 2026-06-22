package prd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CanonSource names the file a derived task came from.
type CanonSource string

const (
	SourceADR      CanonSource = "adr"
	SourceRoadmap  CanonSource = "roadmap"
	SourceArchImpl CanonSource = "arch-impl-order"
	SourceManual   CanonSource = "manual"
)

// DerivedTask is a task candidate extracted from a canon file.
type DerivedTask struct {
	ID         string
	Desc       string
	Acceptance string
	Ref        string
	Source     CanonSource
	// Shipped is true when git/PR evidence suggests this is already done.
	Shipped bool
}

// CanonReader reads and parses all canon sources in a repo directory.
type CanonReader struct {
	repoDir string
}

// NewCanonReader returns a reader rooted at repoDir.
func NewCanonReader(repoDir string) *CanonReader {
	return &CanonReader{repoDir: repoDir}
}

// Derive extracts all DerivedTasks from the repo's canon files.
func (r *CanonReader) Derive() ([]DerivedTask, []string, error) {
	var tasks []DerivedTask
	var refs []string

	// 1. ADR-INDEX.md — Proposed and Partially-In-Force ADRs are unshipped work.
	adrTasks, adrRefs, err := r.parseADRIndex()
	if err == nil {
		tasks = append(tasks, adrTasks...)
		refs = append(refs, adrRefs...)
	}

	// 2. docs/ARCHITECTURE*.md — Neith Triad §2 "Recommended Implementation Order".
	archTasks, archRefs, err := r.parseArchitectureDocs()
	if err == nil {
		tasks = append(tasks, archTasks...)
		refs = append(refs, archRefs...)
	}

	// 3. *_ROADMAP.md / product-roadmap.* — phase/item extraction.
	roadmapTasks, roadmapRefs, err := r.parseRoadmapDocs()
	if err == nil {
		tasks = append(tasks, roadmapTasks...)
		refs = append(refs, roadmapRefs...)
	}

	// Deduplicate canon_refs list.
	refs = uniqueStrs(refs)

	return tasks, refs, nil
}

// --- ADR-INDEX.md parser ---

// adrStatusRe matches a table row: | [ADR-NNN](...) | title | **Status** | date |
var adrStatusRe = regexp.MustCompile(`\|\s*\[ADR-(\d+)\]\(([^)]+)\)\s*\|\s*([^|]+)\|\s*\*\*([^*]+)\*\*`)

// unshippedADRStatuses are statuses that represent accepted-but-not-yet-built ADRs.
var unshippedADRStatuses = []string{
	"proposed",
	"partially in force",
	"in progress",
	"pending",
}

func (r *CanonReader) parseADRIndex() ([]DerivedTask, []string, error) {
	candidates := []string{
		filepath.Join(r.repoDir, "docs", "ADR-INDEX.md"),
		filepath.Join(r.repoDir, "ADR-INDEX.md"),
	}
	path := firstExisting(candidates)
	if path == "" {
		return nil, nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var tasks []DerivedTask
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := adrStatusRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		num := m[1]
		title := strings.TrimSpace(m[3])
		status := strings.ToLower(strings.TrimSpace(m[4]))

		if !isUnshippedADRStatus(status) {
			continue
		}

		id := fmt.Sprintf("ADR-%s", num)
		tasks = append(tasks, DerivedTask{
			ID:         id,
			Desc:       fmt.Sprintf("Implement %s: %s", id, title),
			Acceptance: fmt.Sprintf("%s fully implemented and accepted; tests green", id),
			Ref:        id,
			Source:     SourceADR,
			Shipped:    false,
		})
	}

	relPath, _ := filepath.Rel(r.repoDir, path)
	return tasks, []string{relPath}, scanner.Err()
}

func isUnshippedADRStatus(s string) bool {
	for _, u := range unshippedADRStatuses {
		if strings.Contains(s, u) {
			return true
		}
	}
	return false
}

// --- docs/ARCHITECTURE*.md parser (Neith Triad §2) ---

// implOrderRe matches the Neith Triad section header.
var implOrderRe = regexp.MustCompile(`(?i)##\s+\d*\.?\s*Recommended Implementation Order`)

// ganttTaskRe matches a Gantt task line: "    Section Label  :id, date, Nd"
var ganttTaskRe = regexp.MustCompile(`^\s{4,}([^:]+?)\s*:\s*(active\s*,\s*)?(\w+),`)

// numberedTaskRe matches "1. Something" or "- Something" at top level.
var numberedTaskRe = regexp.MustCompile(`^(\d+\.|[-*])\s+(.+)`)

func (r *CanonReader) parseArchitectureDocs() ([]DerivedTask, []string, error) {
	pattern := filepath.Join(r.repoDir, "docs", "ARCHITECTURE*.md")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		// Fallback: check root level
		pattern = filepath.Join(r.repoDir, "ARCHITECTURE*.md")
		matches, _ = filepath.Glob(pattern)
	}

	var tasks []DerivedTask
	var refs []string
	counter := 1

	for _, path := range matches {
		ts, err := parseArchFile(path, r.repoDir, &counter)
		if err != nil {
			continue
		}
		tasks = append(tasks, ts...)
		rel, _ := filepath.Rel(r.repoDir, path)
		refs = append(refs, rel)
	}
	return tasks, refs, nil
}

func parseArchFile(path, repoDir string, counter *int) ([]DerivedTask, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tasks []DerivedTask
	inImplOrder := false
	inGantt := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Detect section start/end.
		if implOrderRe.MatchString(line) {
			inImplOrder = true
			inGantt = false
			continue
		}
		// Next top-level section ends the impl-order block.
		if inImplOrder && regexp.MustCompile(`^## [^#]`).MatchString(line) && !implOrderRe.MatchString(line) {
			inImplOrder = false
			inGantt = false
			continue
		}

		if !inImplOrder {
			continue
		}

		// Detect Gantt block.
		if strings.Contains(line, "```mermaid") {
			inGantt = true
			continue
		}
		if strings.Contains(line, "```") && inGantt {
			inGantt = false
			continue
		}

		if inGantt {
			// Skip section: and gantt/dateFormat lines.
			if regexp.MustCompile(`^\s*(gantt|dateFormat|section|title)`).MatchString(line) {
				continue
			}
			m := ganttTaskRe.FindStringSubmatch(line)
			if m != nil {
				label := strings.TrimSpace(m[1])
				if label != "" {
					id := fmt.Sprintf("ARCH-%03d", *counter)
					*counter++
					tasks = append(tasks, DerivedTask{
						ID:     id,
						Desc:   label,
						Source: SourceArchImpl,
					})
				}
			}
			continue
		}

		// Plain numbered list inside impl-order.
		if m := numberedTaskRe.FindStringSubmatch(line); m != nil {
			desc := strings.TrimSpace(m[2])
			if desc != "" && len(desc) > 3 {
				id := fmt.Sprintf("ARCH-%03d", *counter)
				*counter++
				tasks = append(tasks, DerivedTask{
					ID:     id,
					Desc:   desc,
					Source: SourceArchImpl,
				})
			}
		}
	}
	return tasks, scanner.Err()
}

// --- Roadmap parser ---

// roadmapPhaseRe matches "## 🛤 Phase N: Title (STATUS)"
var roadmapPhaseRe = regexp.MustCompile(`(?i)#{1,3}\s+(?:[^\s]*\s+)?Phase\s+(\d+)[:\s]+(.+)`)

// statusMarkerRe detects a phase status like (COMPLETE ✅) or (ACTIVE 🚧) or (NEXT)
var statusMarkerRe = regexp.MustCompile(`(?i)\((COMPLETE|DONE|SHIPPED|ACTIVE|NEXT|PENDING|PLANNED|IN.PROGRESS)[^)]*\)`)

func (r *CanonReader) parseRoadmapDocs() ([]DerivedTask, []string, error) {
	candidates := findRoadmapFiles(r.repoDir)
	var tasks []DerivedTask
	var refs []string
	counter := 1

	for _, path := range candidates {
		ts, err := parseRoadmapFile(path, r.repoDir, &counter)
		if err != nil {
			continue
		}
		tasks = append(tasks, ts...)
		rel, _ := filepath.Rel(r.repoDir, path)
		refs = append(refs, rel)
	}
	return tasks, refs, nil
}

func findRoadmapFiles(repoDir string) []string {
	patterns := []string{
		"*_ROADMAP.md",
		"*-roadmap.md",
		"product-roadmap.*",
		"ROADMAP.md",
	}
	var found []string
	for _, pat := range patterns {
		ms, _ := filepath.Glob(filepath.Join(repoDir, pat))
		found = append(found, ms...)
	}
	// Also check docs/ subdirectory.
	for _, pat := range patterns {
		ms, _ := filepath.Glob(filepath.Join(repoDir, "docs", pat))
		found = append(found, ms...)
	}
	sort.Strings(found)
	return found
}

func parseRoadmapFile(path, repoDir string, counter *int) ([]DerivedTask, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tasks []DerivedTask
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := roadmapPhaseRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		phaseNum := m[1]
		rest := strings.TrimSpace(m[2])

		// Strip trailing parenthetical status markers from the description.
		desc := statusMarkerRe.ReplaceAllString(rest, "")
		desc = strings.TrimSpace(desc)
		if desc == "" {
			continue
		}

		// Determine shipped status from the marker.
		shipped := false
		if sm := statusMarkerRe.FindString(rest); sm != "" {
			upper := strings.ToUpper(sm)
			if strings.Contains(upper, "COMPLETE") || strings.Contains(upper, "DONE") || strings.Contains(upper, "SHIPPED") {
				shipped = true
			}
		}

		id := fmt.Sprintf("RM-P%s-%03d", phaseNum, *counter)
		*counter++
		tasks = append(tasks, DerivedTask{
			ID:      id,
			Desc:    fmt.Sprintf("Phase %s: %s", phaseNum, desc),
			Source:  SourceRoadmap,
			Shipped: shipped,
		})
	}
	return tasks, scanner.Err()
}

// --- helpers ---

func firstExisting(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func uniqueStrs(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
