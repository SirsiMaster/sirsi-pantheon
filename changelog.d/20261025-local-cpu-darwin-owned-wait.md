Local CPU lifecycle: accept macOS's signed UID process-table representation while
refusing foreign group members and malformed rows. Observe exact owned-child
termination through public non-reaping libSystem waitid; never infer exit from
libproc absence. Preserve the pre-release identity, quiescence/reap/absence gate
and uncertainty debt. Adds inert SDK-ABI, ECHILD, identity and parser controls.
The original1c native-true failure/debt is retained; native requalification and
the broader original Tier1/functional Stack Lab obligation remain open.
