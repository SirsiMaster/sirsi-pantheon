Ma'at diagnostic request validation now reads flag presence from the executing
Cobra command, avoiding a command-initialization cycle. Explicit empty request
identities and incompatible operation selectors retain the same refusals before
observation or submission. Request wire formats and local memory policy are
unchanged.
