- fix(router): the wake loop now logs a bounded tail of a failed consumer's
  output. `dispatchConsumer` left `cmd.Stdout`/`cmd.Stderr` nil, so `exec.Cmd`
  wired both to `/dev/null` and destroyed the cause of every dispatch failure at
  the source — 3843 of 4082 dispatches across 8 lanes exited 1 carrying nothing
  but `exit status 1`, and `claude-finalwishes` spent a day respawning every 60s
  behind a 1.4 MB log of it. Capture is an `*os.File` pipe, not an `io.Writer`:
  exec hands a file through to the child directly and starts no copier, so a
  setsid-detached consumer's lingering grandchildren cannot hold `cmd.Wait` open
  and stall re-dispatch. The last 4 KB is kept, and silence is reported as
  `(no output)` rather than an empty field. This makes the failure legible; it
  does not fix it.
