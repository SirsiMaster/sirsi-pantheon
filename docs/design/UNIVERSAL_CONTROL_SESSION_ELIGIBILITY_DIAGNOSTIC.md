# Design: Universal Control session-eligibility diagnostic (read-only)

**Status:** proposed, design only, no code · **Task:** `universal-control-session-eligibility-diagnostic` · **Author:** ra · **Date:** 2026-11-24
**Owner of the underlying incident record:** `sirsi-hardware-admin` (SHA)

---

## 0. Provenance and what this document does not know

The ask was recovered from router item `20261027-223745-ra-sirsi-hardware-admin-uc-later-recurrence-owner-channel2-recovery-preserved-separa` (closed 2026-11-03; the short id `20261027-223745` does not resolve, the full id does). That item does **not** contain the original diagnostic request. It only names the task as "the existing canonical UC diagnostic task" and records one incident:

- all three M5 Universal Control (UC) switches were ON, the pointer crossed Studio/M5 but the keyboard stayed on Studio;
- SHA's read-back found the USB Apple keyboard on Studio and the MX Master (Bluetooth) connected to the M5;
- the owner moved the mouse to its Easy-Switch channel 2 (Studio) and it worked. That is a recovery, not proof of prevention.

The task registry (SHA's ledger, `universal-control-recurrence-20261011`) adds that a sandboxed agent session could not run `log show`, `sysctl kern.boottime` or `pgrep`, and that `defaults read com.apple.universalcontrol` returned "Domain not found" there, which "does not prove Universal Control is disabled in the owner's Aqua session". It also names a draft Pantheon PR 1412 (not on `origin/main`; not read).

The repo itself contains nothing on UC (`grep -i "universal control"` over `origin/main` returns no match). Everything below that is about Apple behaviour is therefore **an assumption to verify on a real host**, marked `[A]`. The title's word "session" is read as "execution context on a Mac" (see section 1); if the original ask meant something else, this document needs revising, and that is the first open question.

No Mac setting was read or changed to write this.

## 1. Question and the one claim the tool may make

**Question.** From a given execution context on a given Mac, can that Mac currently take part in UC as the source of the pointer/keyboard, and if not, why not?

**Eligibility is a property of a context, not of a Mac.** `[A]` UC runs as a per-user process (`UniversalControl`, with `sharingd`/`rapportd` help) inside the console user's GUI (Aqua) login session. A context outside that session (ssh, a launchd daemon, a sandboxed agent such as the one recorded above) is not UC-eligible and, more importantly, **cannot observe UC state**.

So the diagnostic returns one of three verdicts per probe, never a boolean:

| verdict | meaning |
|---|---|
| `eligible` | every probe that this context can run passed, and no required probe was unobservable |
| `ineligible:<reason>` | a probe positively failed (reason code below) |
| `unknown:<probe>` | the probe could not be observed from this context |

Rule A35 shape: `unknown` is never rolled up into `eligible` or `ineligible`. A sandboxed context must say `unknown:sandboxed`, not "UC is off".

## 2. Ineligibility reason codes and how each is detected (read-only)

Each probe is a read of OS state through an injected provider (A16): a function that returns a typed value or `ErrUnobservable`. No probe writes, toggles, pairs, unpairs or reconnects.

| code | what it asserts | candidate read-only source `[A]` |
|---|---|---|
| `not-console-session` | this process is not in the logged-in user's GUI session | compare `stat -f %Su /dev/console` to the process uid; `launchctl managername` is `Aqua` |
| `sandboxed` (verdict `unknown`) | process listing and log access are denied | the first `pgrep`/`log show` returns the sandbox error recorded in SHA's task |
| `screen-locked-or-asleep` | session is locked or display asleep | `CGSessionCopyCurrentDictionary` lock key; `pmset -g assertions` |
| `uc-disabled` | explicit disable key set for this host | `defaults -currentHost read com.apple.universalcontrol Disable` (SHA's record says M1 reads `1`; M5 and Studio expose no such key, so "key absent" means "not explicitly disabled", not "enabled") |
| `uc-process-absent` | `UniversalControl` is not running in the session | `pgrep -x UniversalControl` (presence proves nothing about input working, as SHA recorded) |
| `peer-not-nearby` | no paired peer reachable over the UC transport | Bluetooth and AWDL/Wi-Fi presence; SHA's earlier recovery described ConnectionReady over AWDL |
| `account-mismatch` | peers are not signed in to the same Apple ID | cannot be read safely from a context; mark `unknown` unless a local, supported read exists |
| `input-device-owner-mismatch` | the keyboard or pointer is physically attached to a different host than the one the pointer crossed to | `ioreg`/`system_profiler SPBluetoothDataType`/`SPUSBDataType` read on each host: which host holds the USB keyboard, which host holds the Bluetooth mouse |

The last row is the one the recorded incident actually turned on (keyboard on Studio, mouse connected to the M5). It is the only probe that needs two hosts; it is a join of two single-host reads, never a remote command.

## 3. Output

One JSON object per host per run (plus the existing human table):

```json
{"host":"m5","context":"console-aqua|ssh|daemon|sandbox","probes":[{"name":"uc-disabled","state":"pass|fail|unknown","detail":"..."}],"verdict":"eligible|ineligible:uc-disabled|unknown:sandboxed"}
```

Two-host join is a separate pure function over two such objects and reports `input-device-owner-mismatch`.

## 4. Non-goals and safety limits (from the incident record)

- No forced toggle, no watchdog or reset loop, no reboot, no hub or drive disconnect, no unpairing.
- No change to display sleep, lock, or helper pause; attended input policy and all host sessions are preserved.
- No claim of root cause. The diagnostic classifies the state at one instant; it does not prove why UC dropped (`universal-control-recurrence-20261011` stays open).
- M1 is no longer excluded (the registry says the owner later requested M1 participation), but this tool never acts on a host; it only reads.

## 5. Where it would live (when built, not now)

SHA owns the evidence and Mac settings; Pantheon would host only a pure verdict function with injected providers, following the existing side-effect-injection pattern (Rule A16, A21) and a `--json` verb per Rule A10. Build is a separate sprint with SHA's review; this note authorizes none of it.

## 6. Open questions for SHA

1. Is "session" the Aqua-context meaning used here, or something else in the original ask?
2. Is draft PR 1412's service diagnostic the intended host for this, or a separate verb?
3. Which of the `[A]` read sources actually work on macOS 27 for a console-session process? Each needs one observed run on a real host before it is trusted, plus a negative control (a context where the probe must read `unknown`).
