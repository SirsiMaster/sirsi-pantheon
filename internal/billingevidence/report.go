// Package billingevidence reports retained aggregate observations. It does not
// query a provider, enforce spending, or turn delayed billing into admission.
package billingevidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"
)

const Schema = "sirsi-billing-observation-v1"

// Observation is an operator-supplied aggregate, not a credential or an
// authenticated provider receipt. Complete means the named window's query and
// pagination completed, not that a delayed export contains all accrued costs.
type Observation struct {
	Schema         string    `json:"schema"`
	BillingAccount string    `json:"billing_account"`
	ExportTable    string    `json:"export_table"`
	Currency       string    `json:"currency"`
	WindowStart    time.Time `json:"window_start"`
	WindowEnd      time.Time `json:"window_end"`
	ExportAsOf     time.Time `json:"export_as_of"`
	CollectedAt    time.Time `json:"collected_at"`
	Complete       bool      `json:"complete"`
	AmountMicros   *int64    `json:"amount_micros"`
}

type Report struct {
	Status          string       `json:"status"`
	Reason          string       `json:"reason"`
	ObservedAt      time.Time    `json:"observed_at"`
	Observation     *Observation `json:"observation"`
	ExportAgeSecs   *int64       `json:"export_age_seconds"`
	AvailableBudget *int64       `json:"available_budget_micros"`
	CreditBalance   *int64       `json:"credit_balance_micros"`
	Enforcement     string       `json:"enforcement"`
	Authority       string       `json:"authority"`
}

// Decode rejects unknown/duplicate fields, trailing JSON and oversized input;
// otherwise a later amount/complete key could silently replace an earlier one.
func Decode(r io.Reader) (*Observation, error) {
	data, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("billing observation exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("billing observation must be a JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errors.New("invalid observation field")
		}
		switch name {
		case "schema", "billing_account", "export_table", "currency", "window_start", "window_end", "export_as_of", "collected_at", "complete", "amount_micros":
		default:
			return nil, fmt.Errorf("unknown or noncanonical observation field %q", name)
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, fmt.Errorf("duplicate observation field %q", name)
		}
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return nil, err
		}
		fields[name] = raw
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing billing observation JSON")
	}
	// Re-encode once to use the standard strict struct decoder after duplicate
	// checking; only the explicit schema's aggregate fields are accepted.
	canonical, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	strict := json.NewDecoder(bytes.NewReader(canonical))
	strict.DisallowUnknownFields()
	var observation Observation
	if err := strict.Decode(&observation); err != nil {
		return nil, err
	}
	return &observation, nil
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Assess preserves a measured zero, but missing/partial/malformed input is
// unknown. Even recent reported spend is delayed evidence, never a live cap.
func Assess(o *Observation, now time.Time, maxAge time.Duration) (Report, error) {
	r := Report{Status: "unknown", Reason: "No aggregate export observation supplied", ObservedAt: now.UTC(),
		Enforcement: "none", Authority: "operator-supplied aggregate; not independently authenticated provider evidence"}
	if now.IsZero() || maxAge <= 0 {
		return r, errors.New("current time and positive maximum export age required")
	}
	if o == nil {
		return r, nil
	}
	if o.Schema != Schema || o.BillingAccount == "" || o.ExportTable == "" || !currencyPattern.MatchString(o.Currency) {
		return r, errors.New("billing schema, account, export table and ISO currency required")
	}
	if o.WindowStart.IsZero() || !o.WindowEnd.After(o.WindowStart) || o.CollectedAt.IsZero() || o.ExportAsOf.IsZero() ||
		o.WindowEnd.After(o.CollectedAt) || o.CollectedAt.After(now) || o.ExportAsOf.After(o.CollectedAt) {
		return r, errors.New("invalid or future billing window/export/collection time")
	}
	r.Observation = o
	age := int64(now.Sub(o.ExportAsOf).Seconds())
	r.ExportAgeSecs = &age
	if !o.Complete || o.AmountMicros == nil {
		r.Reason = "Incomplete aggregate or missing amount; no zero-spend or remaining-budget inference"
		return r, nil
	}
	r.Status = "reported"
	r.Reason = "Complete retained aggregate for named window only; accrued spend and available budget remain unknown"
	if now.Sub(o.ExportAsOf) > maxAge {
		r.Status = "delayed"
		r.Reason = "Export observation exceeds maximum age; retained amount is not current spend or available budget"
	}
	return r, nil
}
