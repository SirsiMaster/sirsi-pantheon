# Failure-memory bootstrap diagnostics

Distinguish bootstrap-lock open failures from retained-parent sync failures,
including the fixed lock name, descriptor and creation mode while preserving
the underlying error with wrapping. This adds diagnostic context only: no
retry, error waiver, locking-order or durability-policy change. The earlier
full-run ENOENT remains unresolved pending identification of its syscall.
