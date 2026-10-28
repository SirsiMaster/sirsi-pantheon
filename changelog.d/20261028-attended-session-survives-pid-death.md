### Fixed

- A Claude/Codex session that resumes with a new process no longer drops off the router. The reaper used to retire an attended session's thread as soon as its old PID died, so the resumed session's heartbeats were refused and re-registering minted a second thread. Attended session-keyed threads now follow the 2-hour session lease (the same rule PID-less sessions already had). Re-registering with the same session ID renews the original thread. Workers keep the immediate OS-truth reap.
