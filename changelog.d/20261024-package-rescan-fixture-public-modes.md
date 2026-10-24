# Package namespace-rescan fixture modes

Explicitly set the late-entry fixture's signature directory to 0755 and
CodeResources file to 0644 so a restrictive caller umask does not prevent
the test from reaching its intended namespace-rescan refusal. Production
mode checks and the negative assertion are unchanged.
