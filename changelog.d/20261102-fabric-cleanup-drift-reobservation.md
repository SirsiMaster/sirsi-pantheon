Retain the fabric leader's unreaped custody when group observation drifts after TERM, retry within bounded grace and KILL windows, and leave unresolved state as cleanup debt. Reobserve WNOWAIT when a leader exits between its wait receipt and the group snapshot, and correct the fake exec-failure group fixture. Refs: `docs/ADR-054-ONE-HORUS-UNIFIED-AGENT-FABRIC.md`, `scripts/maat/README.md`, `scripts/maat/admitted-run.py`, `scripts/maat/test_admitted_run.py`.

Treat a timed-out non-Darwin process-group observation as uncertainty during the same bounded cleanup sequence.
