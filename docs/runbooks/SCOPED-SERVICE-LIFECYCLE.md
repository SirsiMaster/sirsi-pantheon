# Scoped native service lifecycle

Classification: platform foundation. This is cooperative native maintenance,
not a security sandbox against the operating-system account owner.

Stack Lab library: `maat-scoped-service-lifecycle` in
`contracts/stacklab/maat-system-one-recipe-v1.json`, owned by `stacklab.wing.maat`.
The Ma'at component catalog and traceability matrix link this recipe. Source,
adversarial tests and safe upgrade boundaries travel with the release candidate;
installed service/application proof is recorded separately, not inferred from
the presence of this catalog entry.

Corrective extensions retain those original profiles: the exact root-relay
recipe is `docs/stacklab/pantheon-pt/canon/ROOT_RELAY_REFRESH.md`; the exact
GUI-owner Horus Before-to-After configuration conversion is
`docs/stacklab/pantheon-pt/canon/HORUS_SELECTED_REFRESH.md`. Horus requires the
additional complete conversion-plan digest; it does not weaken the original
profiles' same-artifact readback or authorize bulk daemon installation.

`sirsi launchd restart <label>` is a read-only JSON plan by default. Its initial
closed scope is Apollo observer, chat and Gemma broker user agents, and Mercury
monitor/chip system daemons. It does not enumerate/restart the fabric, enable a
disabled job, signal a caller-provided PID, or invoke administrator prompts.
System daemons require an already authorized privileged controller; this command
does not acquire privilege.

The owning application first drains active work and retains its exact artifact
backup and application-health recovery procedure. In particular, do not restart
the broker during an active model request. `--drained` records the caller's
assertion; it is not an automatic proof that every application request drained.

After inspecting the plan, apply requires `--apply --drained`,
`--expected-pid`, `--expected-start`, `--expected-plist-sha256` and
`--expected-program-sha256`. The controller rechecks the entire instance before
one label is retired, then requires the same artifact digests and a new running
instance. Running launchd state is not an application-health claim. The owning
application must verify its actual health endpoint and recover from its retained
backup if restart or health fails. A failed restart is explicitly reported;
the command never hides it by refreshing unrelated services.

Apply requires an explicit `lifecycle_services` registry grant plus the actual
registered native actor/thread/PID-start identity on the execution host. Agent
names and environment variables alone do not establish runtime ownership.
The bounded own-thread RPC must be available on client and server first
(PR1151). Missing compatibility refuses maintenance without a global thread
list or local database fallback. Roll out the compatible server and verify old
client lifecycle first, then roll compatible clients and this controller.

Artifact checks bind regular-file descriptors and reject symbolic-link leaves
and noncanonical parents. They detect ordinary replacement during inspection;
they do not exclude an independently authorized OS owner racing launchd after
the final check. Program hashing does not qualify script dependencies or replace
the signed package/admission process. Stable deployment, signing, model-drain,
health and rollback evidence remain required for each actual service cutover.

This source change has no installed-service effect. Native deployment evidence
must name the installed build, selected label, pre/post instances, health,
preserved unrelated jobs, and any recovery debt. Never report source fixtures
as a completed native rollout.

The confirmed native Pantheon lead is declared `attended: true` and its generic
background consumer has no `worker_answers` grant. A long build/aged heartbeat
must not silently authorize a competing implementation writer. Deliberately
delegated helpers use separate identities and fenced tasks. This is one-lane
ownership, not an exclusive machine reservation or a restriction on other lanes.
