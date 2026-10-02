# Reads router items as JSONL slurped (-s) with --arg pr <number>. Emits the id of
# the router item carrying the CURRENT rejection of that PR, or nothing.
# Current = the latest verdict-bearing review/decision item that names the PR; a
# later ACCEPT/PASS/APPROVE item for the same PR clears an earlier rejection.
# Verdicts are read from titles, so this errs toward blocking (an override exists).
def names_pr: test("(^|[^0-9A-Za-z])(PR|pull)[ #-]*" + $pr + "([^0-9]|$)"; "i");
def verdict:
  if test("CHANGES[ _-]?(REQUIRED|REQUESTED)|REJECT"; "i") then "reject"
  elif test("ACCEPT|PASS|APPROVE"; "i") then "accept"
  else empty end;
[ .[] | select(.type == "review" or .type == "decision")
      | select(.title | names_pr)
      | {id, opened, v: (.title | verdict)} ]
| sort_by(.opened) | last
| select(. != null and .v == "reject") | .id
