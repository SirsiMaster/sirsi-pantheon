package billingevidence

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func observed(now time.Time) Observation {
	zero := int64(0)
	return Observation{Schema: Schema, BillingAccount: "billingAccounts/example", ExportTable: "project.dataset.table",
		Currency: "USD", WindowStart: now.Add(-48 * time.Hour), WindowEnd: now.Add(-24 * time.Hour),
		ExportAsOf: now.Add(-time.Hour), CollectedAt: now.Add(-time.Minute), Complete: true, AmountMicros: &zero}
}

func TestUnknownIsNotZeroOrUnlimited(t *testing.T) {
	now := time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC)
	r, err := Assess(nil, now, 24*time.Hour)
	if err != nil || r.Status != "unknown" || r.Observation != nil || r.AvailableBudget != nil || r.CreditBalance != nil || r.Enforcement != "none" {
		t.Fatalf("invented budget: %+v %v", r, err)
	}
	o := observed(now)
	for _, missing := range []string{"incomplete", "amount"} {
		v := o
		if missing == "incomplete" {
			v.Complete = false
		} else {
			v.AmountMicros = nil
		}
		r, err = Assess(&v, now, 24*time.Hour)
		if err != nil || r.Status != "unknown" || r.AvailableBudget != nil {
			t.Fatalf("%s: %+v %v", missing, r, err)
		}
	}
}

func TestReportedZeroAndDelayedNegativeSpend(t *testing.T) {
	now := time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC)
	o := observed(now)
	r, err := Assess(&o, now, 24*time.Hour)
	if err != nil || r.Status != "reported" || r.Observation.AmountMicros == nil || *r.Observation.AmountMicros != 0 || r.AvailableBudget != nil {
		t.Fatalf("explicit zero lost: %+v %v", r, err)
	}
	negative := int64(-5000000) // Export credits can exceed charges; never clamp.
	o.AmountMicros = &negative
	o.ExportAsOf = now.Add(-25 * time.Hour)
	r, err = Assess(&o, now, 24*time.Hour)
	if err != nil || r.Status != "delayed" || *r.Observation.AmountMicros != negative || r.CreditBalance != nil {
		t.Fatalf("delayed credit: %+v %v", r, err)
	}
}

func TestInvalidObservationRefuses(t *testing.T) {
	now := time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC)
	for _, mutate := range []func(*Observation){
		func(o *Observation) { o.Schema = "" }, func(o *Observation) { o.BillingAccount = "" },
		func(o *Observation) { o.ExportTable = "" }, func(o *Observation) { o.Currency = "usd" },
		func(o *Observation) { o.WindowStart = o.WindowEnd }, func(o *Observation) { o.CollectedAt = now.Add(time.Minute) },
		func(o *Observation) { o.ExportAsOf = now.Add(time.Minute) }, func(o *Observation) { o.WindowEnd = now.Add(time.Minute) },
	} {
		o := observed(now)
		mutate(&o)
		if _, err := Assess(&o, now, 24*time.Hour); err == nil {
			t.Fatalf("accepted invalid observation: %+v", o)
		}
	}
	if _, err := Assess(nil, now, 0); err == nil {
		t.Fatal("accepted nonpositive age")
	}
}

func TestStrictObservationDecode(t *testing.T) {
	o := observed(time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC))
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{"complete":false,"complete":true}`, `{"complete":false,"COMPLETE":true}`,
		`{"amount_micros":0,"Amount_Micros":100}`, `{"unknown":1}`, `{} {}`, `[]`,
		`{"amount_micros":9223372036854775808}`, strings.Repeat(" ", 1<<20) + "{}",
	} {
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted ambiguous input (length %d)", len(input))
		}
	}
}
