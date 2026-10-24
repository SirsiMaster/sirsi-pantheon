package liveness

// The local observer deliberately has no dispatch/store constructor. Ma'at's
// existing per-user journal retains observations, never router delivery claims.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

const LocalWatchBudget = 12 * time.Second

// LocalObservation is metadata, not an inference or service-start result.
type LocalObservation struct {
	Check               string `json:"check"`
	State               string `json:"state"` // observed, unavailable, not-applicable
	Concern             bool   `json:"concern"`
	Detail              string `json:"detail"`
	ConfigurationSHA256 string `json:"configuration_sha256,omitempty"`
}

type LocalProbe struct {
	Check string
	Read  func(context.Context) (LocalObservation, error)
}

// LocalCommand is per-instance injection for fixed native operations, not a
// caller-supplied command, shell, plugin or registry execution interface.
type LocalCommand func(context.Context, string, []byte, ...string) ([]byte, error)

type LocalWatchResult struct {
	LocalRecord      string             `json:"local_record"`
	RaDelivery       string             `json:"ra_delivery"`
	StartedAt        string             `json:"started_at"`
	FinishedAt       string             `json:"finished_at"`
	Observations     []LocalObservation `json:"observations"`
	Verdict          maat.MaatVerdict   `json:"verdict"`
	DecisionEvidence string             `json:"decision_evidence,omitempty"`
}

var localChecks = []string{"local-watch", "launchd-disabled", "menubar", "memory-metadata"}

// RunLocal executes a finite synchronous set. Probe implementations must honor
// ctx; production probes use CommandContext, not abandoned timeout goroutines.
// A fresh caller needs HOME only, not a checkout, registered lane or router DB.
func RunLocal(ctx context.Context, journal maat.DecisionJournal, host string, probes []LocalProbe) (LocalWatchResult, error) {
	result := LocalWatchResult{LocalRecord: "failed", RaDelivery: "not_attempted", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if journal == nil || strings.TrimSpace(host) == "" || len(host) > 256 || len(probes) != len(localChecks) {
		return result, fmt.Errorf("local watch: journal, local host and complete probe set required")
	}
	ctx, cancel := context.WithTimeout(ctx, LocalWatchBudget)
	defer cancel()
	for i, probe := range probes {
		if probe.Check != localChecks[i] || probe.Read == nil {
			return result, fmt.Errorf("local watch: invalid closed probe set")
		}
	}
	for _, probe := range probes {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		probeCtx, stop := context.WithTimeout(ctx, 2*time.Second)
		observation, err := probe.Read(probeCtx)
		if probeCtx.Err() != nil {
			err = probeCtx.Err()
		}
		stop()
		if err != nil {
			// Never copy subprocess stderr, private paths, environment or bodies.
			observation = LocalObservation{Check: probe.Check, State: "unavailable", Detail: "bounded local metadata probe unavailable"}
		}
		if observation.Check != probe.Check || len(observation.Detail) == 0 || len(observation.Detail) > 2048 ||
			(observation.State != "observed" && observation.State != "unavailable" && observation.State != "not-applicable") ||
			(observation.State != "observed" && observation.Concern) {
			return result, fmt.Errorf("local watch: invalid observation for %s", probe.Check)
		}
		result.Observations = append(result.Observations, observation)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(result.Observations)
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(raw)
	id := hex.EncodeToString(digest[:])
	screen := maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "host", Repo: host, Ref: "local-watch-metadata", HeadSHA: id},
		FeatherWeight: 100, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "bounded local metadata observation", Passed: true, Detail: "four typed probes; no generation, network, or repair; scope is not whole-machine health"}}},
		Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "local-watch-v1", Local: true},
	}
	for _, observation := range result.Observations {
		readable := observation.State != "unavailable"
		screen.Floor.Checks = append(screen.Floor.Checks, maat.FloorCheck{Name: observation.Check, Passed: readable, Detail: observation.State + ": " + observation.Detail})
		screen.Floor.Passed = screen.Floor.Passed && readable
		if observation.State == "unavailable" || observation.Concern {
			finding := maat.ScreenFinding{ID: observation.Check, Severity: "major", Category: "host-health", Claim: observation.Detail, Evidence: "local-watch:sha256=" + id, Confidence: 1}
			if observation.State == "unavailable" {
				finding.Confidence = 0
				screen.Confidence = 0
			}
			// Only these existing closed repair workflows may be offered.
			if observation.State == "observed" && observation.Concern {
				switch observation.Check {
				case "local-watch":
					finding.RepairID = maat.SystemOneRepairLivenessWatch
				case "launchd-disabled":
					finding.FixHint = "Review intentional disabled services; confirmed managed recovery is separate from observation."
				}
			}
			screen.Findings = append(screen.Findings, finding)
		}
	}
	result.Verdict, err = maat.Screen(screen)
	if err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	decision, err := maat.RecordSystemOne(journal, "sirsi liveness-watch run --local", result.Verdict)
	if err != nil {
		return result, fmt.Errorf("local watch: retain observation: %w", err)
	}
	result.LocalRecord, result.DecisionEvidence = "recorded", decision.Evidence
	return result, nil
}
