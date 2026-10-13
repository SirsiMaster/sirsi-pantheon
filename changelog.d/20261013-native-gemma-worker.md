### Fixed

- Replaced the shell-owned Gemma router consumer with a native fenced worker:
  warm-SNE preflight before claim, one worker singleton, registered/heartbeated
  consumer identity, token-fenced completion, and retained retry evidence for
  failed inference or reply delivery. Gemma drafts remain nonbinding.
