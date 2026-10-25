The Mercury web renderer no longer retains a prior read's activity lamps when
the current Go projection supplies an empty/null lamp vector. A fresh idle null
vector clears prior lamps; a missing vector is explicitly unavailable rather
than inferred off. Current arrays and existing aggregate eligibility remain
authoritative.

An actual embedded-renderer active-to-idle regression covers null, missing and
empty vectors, zero active cables, idle context, withheld hero cells and stopped
motion without a clock-triggered fetch. Both existing DOM fixtures now implement
the production mount's appendChild call with node ownership/return semantics;
their original cancellation/expiry assertions are retained. The frozen5d53
attempt's four passing Node cases and two pre-render fixture failures are not
reclassified. No new execution, Swift change or whole G1–G5 completion claimed.
