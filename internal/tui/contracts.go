package tui

// Contract types — the exact shapes of the merged `sirsi <verb> --json` outputs
// the console reads (PR #139: vitals, scan, ghosts, activity; plus diagnose).
// These mirror the producer structs (internal/guard, cmd/sirsi) field-for-field
// so the TUI decodes without a translation layer. The TUI reads ONLY these; it
// never re-implements the probing logic — Go stays the brain.

// --- sirsi vitals --json (cmd/sirsi/vitalscmd.go vitalsReport) ---

type vitalsProc struct {
	Name     string `json:"name"`
	PID      int    `json:"pid"`
	RSSBytes int64  `json:"rss_bytes"`
}

type vitalsReport struct {
	Command        string       `json:"command"`
	TotalBytes     int64        `json:"total_bytes"`
	UsedBytes      int64        `json:"used_bytes"`
	FreeBytes      int64        `json:"free_bytes"`
	SwapUsedBytes  int64        `json:"swap_used_bytes"`
	Pressure       string       `json:"pressure"`
	PressureSource string       `json:"pressure_source"`
	Top            []vitalsProc `json:"top"`
}

// --- sirsi scan --json (internal/jackal ScanResult) ---

type scanFinding struct {
	RuleName    string `json:"RuleName"`
	Category    string `json:"Category"` // read: "ai" model-weights are BLOCK, never cleanable
	Description string `json:"Description"`
	Path        string `json:"Path"`
	SizeBytes   int64  `json:"SizeBytes"`
	FileCount   int    `json:"FileCount"`
	Severity    string `json:"Severity"` // safe | caution | warning (jackal.Severity vocab)
	CanFix      bool   `json:"CanFix"`
	Breaking    bool   `json:"Breaking"`
}

type scanReport struct {
	Findings        []scanFinding `json:"Findings"`
	TotalSize       int64         `json:"TotalSize"`
	ReclaimableSize int64         `json:"ReclaimableSize"`
	RulesRan        int           `json:"RulesRan"`
}

// --- sirsi clean --json (output.CommandResult subset) ---

type cleanEvidence struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type cleanNextAction struct {
	Label       string `json:"label"`
	Command     string `json:"command"`
	Description string `json:"description"`
}

type cleanReport struct {
	Command     string            `json:"command"`
	Status      string            `json:"status"`
	Summary     string            `json:"summary"`
	Evidence    []cleanEvidence   `json:"evidence"`
	NextActions []cleanNextAction `json:"next_actions"`
}

// --- sirsi ghosts --json (cmd/sirsi/anubis.go ghostReport) ---

type ghostResidual struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"size_bytes"`
	FileCount int    `json:"file_count"`
}

type ghostApp struct {
	AppName         string          `json:"app_name"`
	BundleID        string          `json:"bundle_id"`
	TotalSizeBytes  int64           `json:"total_size_bytes"`
	TotalFiles      int             `json:"total_files"`
	InLaunchService bool            `json:"in_launch_services"`
	Residuals       []ghostResidual `json:"residuals"`
}

type ghostReport struct {
	Command         string     `json:"command"`
	Summary         string     `json:"summary"`
	GhostCount      int        `json:"ghost_count"`
	TotalWasteBytes int64      `json:"total_waste_bytes"`
	TotalWaste      string     `json:"total_waste"`
	Ghosts          []ghostApp `json:"ghosts"`
}

// --- sirsi activity --json (cmd/sirsi/activitycmd.go activityReport) ---

type activityEntry struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	Target string `json:"target"`
	Bytes  int64  `json:"bytes"`
	Source string `json:"source"`
}

type activityReport struct {
	Command string          `json:"command"`
	LogPath string          `json:"log_path"`
	Count   int             `json:"count"`
	Entries []activityEntry `json:"entries"`
}

// --- sirsi maat casebook --json (internal/maat/casebook View) ---
//
// The terminal console intentionally consumes Ma'at's projected evidence
// journal rather than deriving its own diagnoses. Keep this contract read-only:
// resolution/acceptance remains a separately confirmed action in the native
// Pantheon app or the explicit CLI commands.

type maatResolutionPath struct {
	Kind                 string               `json:"kind"`
	Title                string               `json:"title"`
	Detail               string               `json:"detail"`
	Evidence             string               `json:"evidence"`
	RequiresConfirmation bool                 `json:"requires_confirmation"`
	Steps                []maatResolutionStep `json:"steps"`
}

type maatResolutionStep struct {
	Level                int    `json:"level"`
	Title                string `json:"title"`
	Detail               string `json:"detail"`
	Evidence             string `json:"evidence"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

type maatSystemOneSubject struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	HeadSHA string `json:"head_sha"`
}

type maatSystemOneEscalation struct {
	Reason string `json:"reason"`
}

type maatSystemOneFloorCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type maatSystemOneFloor struct {
	Passed bool                      `json:"passed"`
	Checks []maatSystemOneFloorCheck `json:"checks"`
}

type maatSystemOneModel struct {
	Provider  string `json:"provider"`
	Version   string `json:"version"`
	Local     bool   `json:"local"`
	LatencyMS int    `json:"latency_ms"`
}

type maatSystemOneFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Claim    string `json:"claim"`
	Evidence string `json:"evidence"`
	FixHint  string `json:"fix_hint"`
}

type maatSystemOneVerdict struct {
	Gate          string                   `json:"gate"`
	Confidence    float64                  `json:"confidence"`
	FeatherWeight int                      `json:"feather_weight"`
	Subject       maatSystemOneSubject     `json:"subject"`
	Escalation    *maatSystemOneEscalation `json:"escalation"`
	Floor         maatSystemOneFloor       `json:"floor"`
	Model         maatSystemOneModel       `json:"model"`
	Findings      []maatSystemOneFinding   `json:"findings"`
}

type maatCase struct {
	ID            string                `json:"id"`
	Time          string                `json:"time"`
	Kind          string                `json:"kind"`
	Category      string                `json:"category"`
	Status        string                `json:"status"`
	Priority      string                `json:"priority"`
	Requester     string                `json:"requester"`
	Resource      string                `json:"resource"`
	Affected      string                `json:"affected"`
	Determination string                `json:"determination"`
	Assessed      string                `json:"assessed"`
	Why           string                `json:"why"`
	Evidence      string                `json:"evidence"`
	Resolution    string                `json:"resolution"`
	NextAction    *maatResolutionPath   `json:"next_action"`
	SystemOne     *maatSystemOneVerdict `json:"system_one"`
}

type maatCasebookSummary struct {
	Total    int `json:"total"`
	Open     int `json:"open"`
	Urgent   int `json:"urgent"`
	High     int `json:"high"`
	Resolved int `json:"resolved"`
}

type maatCasebookReport struct {
	Cases   []maatCase          `json:"cases"`
	Summary maatCasebookSummary `json:"summary"`
}

// --- sirsi diagnose --json (internal/guard DiagnosticReport) ---

type diagFinding struct {
	Check      string `json:"check"`
	Severity   int    `json:"severity"` // 0 ok · 1 attention · 2 critical
	Message    string `json:"message"`
	Detail     string `json:"detail,omitempty"`
	Trend      bool   `json:"trend,omitempty"`
	ActiveDays int    `json:"activeDays,omitempty"`
	Fix        string `json:"fix,omitempty"`
	FixKind    string `json:"fixKind,omitempty"` // instant | relief | guidance
}

type diagReport struct {
	Timestamp string        `json:"timestamp"`
	Duration  string        `json:"duration"`
	Findings  []diagFinding `json:"findings"`
}
