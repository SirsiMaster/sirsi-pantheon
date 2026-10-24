# Pantheon native workspace

Mode: Operate. Native SwiftUI desktop application; the menu bar opens the same
workspace. This increment uses Apollo's performance desk and Mercury's Sirsi
visual lineage. Black owns the interface; emerald identifies measured healthy
state and gold identifies navigation, selection and considered action.

## System

Tokens live in `macapp/Sources/SirsiMenubar/Views.swift`, `PantheonTheme`.
Canvas #020605, sidebar #040A08, panel #070E0B, raised panel #0B130F,
text #EDF4EF, secondary #A4C9B9, emerald #6EE7B7, gold #E3C878,
separators #23483B. Native system typography: 27-point introduction,
19-point workspace title, 15-point section headings, 13-point navigation/body,
12-point supporting text. Measurements use tabular numerals. Fixed sidebar
218 points; content remains fluid down to the supported 900-point window.
Separators and a single background elevation establish grouping.

## Responsibilities and routes

Overview presents observed health, pending decisions, the four principal
workspaces, local inspection routes and retained activity. Unread sources stay
unobserved. Top-level choices update the sidebar and header together; object
drill-downs use the existing Nav stack and retain a way back.

Apollo owns inference selection and telemetry. Plans carry machine, engine,
CPU, memory, swap and estate choices to the telemetry destination. Revising a
plan retains that declaration and checks it against the current catalog;
removed machines or routes cannot silently become local defaults.

Mercury consumes the existing canonical `sirsi mercury --json` dashboard read,
which owns the bounded private GET, schema validation, units and producer
freshness assessment. The native view refreshes every five seconds while
visible and independently ages retained samples with a monotonic clock; failed,
expired and future observations cannot remain current, including while a read
is pending. Scene rates are withheld when their canonical direction timestamps
expire. Dashboard v1 does not expose power telemetry, which remains unknown;
the broader native telemetry obligation is still open. Ra lane/machine and
host transport projections remain separate evidence and do not establish
unreported rail reservations or negotiated link capacity.
Lane availability expires after 30 seconds independently of live machine
telemetry. Ra's retained board is likewise not current after a failed read or
30 seconds; its refresh control remains available with populated data. Apollo
accepts metrics as current only with a parseable producer `emitted_at` at most
15 seconds old and no more than two seconds ahead of the local clock. Missing
or expired timestamps yield a recovery state, not a live-session claim.

Ra owns fabric workers, messages, claims and handbacks. Ma’at owns local
System One knowledge, decisions and evidence. An Apollo guidance handoff
displays the original plan context above Ma’at's existing controls. It does
not assert that opening the screen evaluated or repaired that plan.
Confirmed Ma’at case operations retain their in-flight guard and result outside
view lifetime. Returning to a workspace does not authorize duplicate submission;
the exact case/action evidence must be reread before a confirmed operation.

Stack Lab owns recipes/components and qualification evidence. Release remains
the application delivery workspace. Horus, Anubis and Osiris own local health,
scoped storage recovery and checkpoints. All Tools retains the smaller tools.

## Behavior

Command-K focuses workspace search. Search covers names and responsibilities;
Enter opens the first match and Clear recovers from no results. Each workflow
owns its refresh action and preserves edited plans and case context. Overview
and Apollo bind Command-R to their observation reads; refreshing never starts
a storage scan. Each workspace retains its own in-memory drill-down stack;
inactive views unmount and stop their tasks. Selection persists across launches,
without replaying operations or confirmations. Root-local drafts and scroll
restoration are not guaranteed. The Mac
window supports minimize, close and resize and preserves one menu-bar companion.

## Delivery

Classification: commercial-product UI increment to the existing public app.
User: the Mac operator managing local intelligence and connected work. Success:
reach each principal workflow from persistent navigation, preserve selected
context into the next operation, and see evidence/error/next-action states.
Canonical Go commands retain operation authority. SSA owns integration and the
single release train; source/build evidence alone is not an installed release.

Validation: Swift package tests, capacity/route drift controls, window-width
rendering and independent source/design review. Signed packaging, installed
client observations and mixed client/server proof are recorded by SSA with the
release receipt. Source review and candidate harness evidence are not a claim
that this increment has been installed or publicly released.

The isolated native interaction harness uses unchanged product Swift sources
with a separate application entry point, no menu-bar companion and no bundled
CLI. Its window uses the same hosting-size policy as AppDelegate. It verifies
Command-K, search recovery, Return-to-open, actual scroll movement, Back and
per-workspace detail retention. Missing-CLI errors exercise recovery links;
they do not substitute for successful installed operations. The harness reads
the existing Mercury monitor but starts no monitor, model or service.
