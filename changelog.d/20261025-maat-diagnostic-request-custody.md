Bind local diagnostic records to durable, once-consumed request UUIDs before
observation. Add exact read-only pending/completed lookup and confirmed,
writer-serialized request fencing/data-only reconciliation after lost replies;
never resend or infer safety from absence. Preserve historical uncorrelated
records and distinguish fenced-not-recorded from never-submitted outcomes.

Compose a versioned privacy-safe local-host binding with the existing Go/native
diagnostic consumers. Preserve original canonical finding identities for unknown
redacted checks without retaining raw check text. Historical observation-v1
bytes and strict legacy hostname matching remain unchanged. Native correlation
adapter/refresh integration and actual qualification remain separate work.
