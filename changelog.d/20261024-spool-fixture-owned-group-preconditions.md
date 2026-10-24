# Spool fixture group and mode preconditions

Group-trust fixtures now explicitly assign owned temporary directories to
the process's primary group before setting exact permissions, including
setgid, and assert the initial group and mode. This avoids inheriting an
unrelated temporary-parent group or losing required bits to a restrictive
umask. The test group-name resolver follows the process's actual primary gid.

Existing-directory preservation, missing-trust refusal, group migration,
group-aware publication and observation fixtures retain their production
assertions. Production group trust and private-mode policies are unchanged;
no host spool, global umask or test-skip policy is modified.
