# Archive preservation fixture mode

Explicitly set and assert the existing .sirsi fixture directory's 0755 mode
before opening the archive, independent of the caller's umask. Production
privacy, nofollow, existing-mode preservation and link-refusal checks remain
unchanged.
