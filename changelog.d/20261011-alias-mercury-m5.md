- **fix(router):** declared `mercury-m5` as an alias of `mercury`. Two Studio identity notes from
  sirsi-hardware-admin addressed to the undeclared `mercury-m5` sat stranded (no wake mechanism, and
  the reassign guard only lets a recipient or a declared alias successor move them). The alias lets
  `sirsi router drain-aliases` move them to `mercury`, which owns the Studio.
