# ADR: Bare Studio scheduler alias compatibility

Status: accepted bounded source decision by SSA after independent inspection of
producer3ccedd84 and its native regression receipts,2026-10-18. Operational
closure still requires actual canonical service/client deployment and readback.

The native supervisor uses studio while legacy Mercury callers use m5-studio
for one physical Mac. Literal scheduler equality admits two full-share pools.

Use a private comparison key in the shared schedule engine for exactly these
bare names. Keep reservation IDs, resource fields, request/response spelling,
history and runtime/executor host binding unchanged. Do not alias components,
rails, arbitrary casing or other machine names. Capacity readers accept either
legacy key and select the smaller positive setting on disagreement; explicit
setters synchronize both capacity keys. No database migration or preemption.

The canonical transactional server and client fallback share this engine.
Every server replica must carry the fix for operational closure. Tests cover
both directions, historical records, queue/floor/read/coverage/cede behavior,
unrelated names and concurrent canonical store admissions. Signed publication,
CI and deployed receipts remain separate gates. Existing cede/capacity state
write atomicity is unchanged; this decision does not claim to repair it.
