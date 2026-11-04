### Fixed
- A quiet attended session whose process is still running no longer loses its thread when the 2h lease lapses. The reaper checks the process first, so waiting on the owner no longer burns a thread id and forces a re-register.
