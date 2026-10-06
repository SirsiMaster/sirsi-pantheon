### Fixed

- **`TestSenderFloodRejected` no longer fails at the top of the hour.** It capped the send quota at 10 and expected at most 11 items, but ran on the real clock while the quota window is one wall-clock hour: a 500-send loop that straddled the hour got two budgets (10 + 10 + 1 throttle = 21) and failed the pre-push gate during the v0.24.95 release at 06:00 UTC. The test now pins the store clock, and a new test pins the real behavior it tripped on (a new window grants a fresh budget).
