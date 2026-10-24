# Failure-memory bootstrap error metadata

On bootstrap-lock open or parent-sync failure only, capture best-effort
device/inode/link-count/mode/owner metadata for the retained parent and named
lock, plus the opened lock when available. Capture occurs before releasing
owned descriptors; failed metadata probes are reported explicitly. No file
contents are read, and these observations are neither atomic authority nor
a basis for retrying or accepting ENOENT.

The prior full-run initialization failure remains unresolved. Subsequent
passing diagnostic repetitions do not establish a repair or combined pass.
