- **fix(signing):** use existing pinned-host, noninteractive SSH authentication
  consistently for signing preflight, publication, polling, SCP and rsync.
  Unknown host keys fail without being learned; missing authentication cannot
  open a later password/MFA prompt. Preserve signature verification and signing
  protocol; add isolated file/app, refusal, foreign-signature and permissive-policy
  negative fixtures. Real signing and installed Apollo delivery remain separately
  qualified.
