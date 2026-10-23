### Fixed
- Carry the native launcher's finite CPU request into `sirsi maat reserve --cores`
  and the canonical reservation. A first-arriving bounded CPU-only build no
  longer owns the model slot merely because its complete request was granted.
- Preserve quiet/loaded, unknown-share, unbounded, oversized and memory-request
  conflict behavior. No existing reservation, foreign process or workload is
  stopped or rewritten. CPU environment masks remain cooperative, not kernel
  CPU-affinity enforcement.
- When a finite CPU ask and a model coexist, current-holder/conflict projections
  select the actual full model/measurement owner instead of hiding it behind
  the first CPU row. Both original reservations remain intact.

Refs: docs/stacklab/maat/canon/PLAN.md,
docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md; Ra source6735d893 and
SSA independent released124 reproduction/integration.
