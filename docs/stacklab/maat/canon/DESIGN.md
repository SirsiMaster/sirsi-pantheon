# Design — Ma'at System One

The canonical flow is `observe -> assess -> append decision -> Casebook -> CLI/Horus API`. `internal/maat` owns assessment semantics; Casebook derives a read-only view. Unknown status, unavailable journal input, and unsupported writes return errors rather than synthetic success.
