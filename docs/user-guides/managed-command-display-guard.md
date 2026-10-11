# Managed command display guard

A managed command tool refuses recognized direct display-power operations before
starting a shell. Read-only power queries and tools without commands continue.
The refusal exits with code 2 and explains that desktop custody is required.
Generic shell dispatch never accepts a custody grant as an override.

This source candidate expands the hook matcher beyond Bash. It needs independent
review, CI and native installation qualification before deployment. The installer
refuses conflicting installed files; it is never invoked by a product UI.

The hook cannot inspect a pure computer-use click. Those operations require their
own revocable computer-use authority. This change does not certify protection
across the desktop or whole host. No real power command is used as a test canary.
