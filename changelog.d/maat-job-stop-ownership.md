Ma’at's native supervised front door now refuses direct and shell-wrapped
process-stop commands before registration/admission/spawn. A compute grant is
not lifecycle authority over a matching name, PID, or same-lane service instance.
Normal supervisor-owned child teardown is unchanged. This guards common
accidental stops; arbitrary scripts and unrestricted same-UID shells are not
an OS isolation boundary. Existing active jobs are not restarted by this change.
