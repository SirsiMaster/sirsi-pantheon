- **fix(maat):** the guarded-run supervisor (`sirsi-maat-run`) now records the
  run's exit status when it releases its reservation (`maat release
  --exit-code`), so `maat:` receipts verify instead of being refused as
  unverifiable. Against an older `sirsi` that lacks the flag it releases as
  before.
