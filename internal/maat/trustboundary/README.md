# `internal/maat/trustboundary` — the Trust-Boundary gate (ADR-076)

Ma'at's mechanical half of the Trust-Boundary Law. `cmd/sirsi/maatgate.go` is the
CLI (`sirsi maat gate`); this package does the work.

## Architecture

| File | Role |
|---|---|
| `gate.go` | `Gate.Run` — four steps, each with its own pass/fail/skip; `RunRanges` aggregates every pushed ref; `RangesFromPrePush` parses git's pre-push stdin; `blob` reads `rev:path` and distinguishes *absent* (`errAbsent`) from a read failure |
| `lint.go` | `LintBlobs(read, paths)` — rule heuristics A–H over Go (go/ast), TS/JS (regex) and shell/YAML; `LintTree`/`LintFiles` are the working-tree survey wrappers |
| `report.go` | `Report()` — the A–H checklist skeleton (`--report`) |
| `testdata/` | one minimal fixture per class with `// want X` markers; `lint_test.go` asserts exactly those letters fire and nothing else |

**Nothing in range mode is read from the working tree.** The verifier that runs is `git show base:scripts/verify-commit-traceability.sh` — the copy the remote already trusts — extracted beside the base's exemption list; if the script differs between base and head (or exists at only one) the gate fails ("verifier changed in range"), so a push cannot attest itself. The lint reads `git show head:<path>` for every path in `git diff --name-only -z --no-renames --diff-filter=d base head` (every path existing at head that differs from base; a rename-with-edit is an added file; `-z` keeps names with spaces/non-ASCII exact — a C-quoted name reaching the lint is an error). The working tree is consulted only in survey mode (no range). A path named by the diff that cannot be read at the head is an error, not a skip.

## Contract (what fails)

- `traceability`: the BASE verifier's own exit status. Script changed/added/removed in range → `fail` (separate review). Absent at both → `skip` (reported). Read error → `fail`.
- `exemption-growth`: `set(head) ⊆ set(base)` over 40-hex lines; any added hash → `fail` naming it. Absent at head → `skip`; absent at base with present at head → every entry is an addition → `fail`. Read error → `fail`.
- `secrets`: gitleaks exit status over `base..head`; binary absent → `skip` (CI remains authoritative).
- `trust-boundary-lint`: any finding → `fail`; unreadable blob or unparsable Go → `fail` (unknown never passes).
- `RangesFromPrePush`: every branch ref with a non-zero local sha yields a range; tags/deletions none; a new branch with no merge-base against `origin/main`/`origin/master`/`origin/HEAD` is an **error** (fail closed). `RunRanges` ORs failures across refs and tags each step with its ref.

## Heuristics — exact limits

These are tripwires for review, not proofs. Each is file-local; none follows calls across files or types.

| Rule | Go | TS/JS | Shell/YAML |
|---|---|---|---|
| **A** | identifier assigned from `strconv.Atoi/ParseInt/ParseUint/ParseFloat`, propagated through plain `=`/`:=`/`var` for **3 fixed passes** (so `max = n` is caught, `a→b→c→d` is not), then used in `make(T, n)`, a `for` condition, or `for range n` | `Object.keys(...).map(Number|parseInt)` or `Number/parseInt(k)` where `k` is a `for…in` / `Object.keys().forEach|map|…` key, propagated through `x = y` / `x = Math.max(..y..)` for 3 passes, then used in `Array.from({length: n})`, `new Array(n)`, `Array(n)` or `; i < n ;` | — |
| **B** | `InsertPages/ImportPages/AddPages/AppendPages/CopyPages/InsertPage/ImportPage/AddPage/AppendPage/MergeFile/AppendFile` called inside any `for`/`range` body | `.addPage/.copyPages/.insertPage/.embedPage/.addPages(` on a line inside an open `for(`/`.forEach(`/`.map(`/`while(`/`.reduce(` body (brace-depth tracking) | — |
| **C** | struct whose name ends `Request|Req|Input|Body|Payload|Params|Submission` with a field name or tag matching `hash|receipt|checksum|digest|signature|provenance|attest|generatedby` | `interface|type X(Request|…)` body fields matching the same | — |
| **D** | file comment `// trust-boundary-gate: F1, F2`: every function taking `http.Request`/`http.ResponseWriter`/`connect.Request`/`gin.Context`/`echo.Context`/`context.Context` must call each listed name (by callee name) | same directive: every `export function`/`export const x = (` body must contain `F1(` and `F2(` | — |
| **E** | `Set/Create/Update(...)` with `firestore.MergeAll` and a map composite literal, or a variable named `*map*`, `*data`, `*Data`, `*fields`, `*Fields` | `setDoc/updateDoc/addDoc/set/update/add/create(` whose argument region has `merge: true` and a `...spread` or a bare variable document | — |
| **F** | string keys matching `ssn|social_security*|tax_id|ein|dob|date_of_birth|birth_date|answers|form_answers|form_data|passport*|drivers_licen[cs]e*|bank_account*|account_number|routing_number|medical*|diagnosis|hipaa*|password|pin|card_number` inside a `Set/Create/Update` composite literal | same keys inside the write's argument region (≤ 800 chars or first `)`) | — |
| **G** | `x.ReplaceAllString(v, "")` where `v`'s identifiers match `amount|money|price|cents|total|balance|dollar|fee|cost|usd|payment|refund|charge` | `.replace(/\D/g,'')` / `/[^0-9]/g` / `/[^\d]/g` with `''` when the line or the one above matches the money names | — |
| **H** | `depth|level|nesting` identifier compared `< > <= >=` to an integer literal in `(0, 64)` | `*depth*|*level*|*nesting*  >= N` or `MAX_DEPTH|maxDepth|MAX_NESTING|maxNesting = N` with `0 < N < 64` | a line containing `verify|check|test|lint|gate|audit … \| tail|head|grep|wc|cat` in a file without `pipefail`; any `git push … --no-verify` |

Skipped paths: `*_test.go`, `*.test.*`, `*.spec.*`, `*.gen.ts`, `*.pb.go`, `*.connect.go`, `*.d.ts`, and anything under `.git node_modules vendor dist build testdata .claude gen coverage ios android`.

**Allowlist:** `trust-boundary: <reason>` on the flagged line or the line directly above silences every finding on that line. The reason is reviewed like code.

## Known limitations

- Taint tracking is intraprocedural and limited to three assignment passes; a client number laundered through a helper function is not seen.
- TS heuristics are regex over text; a loop body split across unusual brace layouts can be mis-scoped either way.
- Rule D depends on the file-level directive; files without it are not checked for sibling gates.
- Rule F keys on field *names*; a PII value under an innocuous key is not seen.
- `secrets` and the hook's `sirsi` presence degrade to visible skips — CI remains the authoritative gate for both.
