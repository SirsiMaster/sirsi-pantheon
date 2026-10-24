# JSONL slurped (-s), --arg repo owner/name --arg pr number --arg head full-SHA.
# Return ALL unresolved rejection ids. Titles can retain historical holds, but
# never clear them. Only a closed binder-specific declaration in existing body,
# from the same canonical sender, may resolve an exact rejection on this head.
def names_pr($n):
  test("(^|[^0-9A-Za-z])(PR|pull)[ #-]*" + $n + "([^0-9]|$)"; "i");
def rejects: test("CHANGES[ _-]?(REQUIRED|REQUESTED)|REJECT"; "i");
def declaration:
  (.body | if type == "string" then . else "" end | split("\n")
    | map(if endswith("\r") then .[:-1] else . end)) as $lines
  | [range(0; $lines | length) | select($lines[.] == "```sirsi-bind-verdict-v1")] as $starts
  | if ($starts | length) != 1 then null
    else
      $lines[$starts[0]:($starts[0] + 7)] as $block
      | if ($block | length) != 7 or $block[6] != "```" then null
        else (try {
          repo: ($block[1] | capture("^repo: (?<value>[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)$") | .value),
          pr: ($block[2] | capture("^pr: (?<value>[1-9][0-9]*)$") | .value),
          head: ($block[3] | capture("^head: (?<value>[a-fA-F0-9]{40})$") | .value),
          verdict: ($block[4] | capture("^verdict: (?<value>ACCEPT|REJECT)$") | .value),
          resolves: ($block[5] | capture("^resolves: (?<value>-|[A-Za-z0-9][A-Za-z0-9._:-]*(?:,[A-Za-z0-9][A-Za-z0-9._:-]*)*)$") | .value)
        } catch null) // null end
      | if . == null then null
        elif .verdict == "REJECT" and .resolves != "-" then null
        elif .verdict == "ACCEPT" and .resolves == "-" then null
        else {repo: (.repo | ascii_downcase), pr, head: (.head | ascii_downcase), verdict,
              resolves: (if .resolves == "-" then [] else .resolves | split(",") end)}
        end
    end;
def relevant($d):
  if $d == null then .title | names_pr($pr)
  # A declaration contradicting a target-PR title cannot scope a hold away.
  elif (.title | names_pr($pr)) and $d.pr != $pr then true
  else $d.repo == ($repo | ascii_downcase) and $d.pr == $pr end;
def identity: if type == "string" then test("^[A-Za-z0-9][A-Za-z0-9_.:-]*$") else false end;

if ($repo | test("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")) and
   ($pr | test("^[1-9][0-9]*$")) and ($head | test("^[a-fA-F0-9]{40}$")) then .
else error("router bind requires an exact repository, PR and full head SHA") end
| [ .[] | select(.type == "review" or .type == "decision")
    | .title = (.title | if type == "string" then . else "" end)
    | declaration as $d
    | . + {bind_declaration: (if (.title | rejects) and $d.verdict == "ACCEPT" then null else $d end)} ] as $items
| [ $items[] | . as $rejection
    | select((.title | rejects) or .bind_declaration.verdict == "REJECT")
    | select(relevant(.bind_declaration))
    | select(any($items[];
        .bind_declaration != null and .bind_declaration.verdict == "ACCEPT"
        and (.title | rejects | not)
        and .bind_declaration.repo == ($repo | ascii_downcase)
        and .bind_declaration.pr == $pr
        and .bind_declaration.head == ($head | ascii_downcase)
        and (.from | identity) and ($rejection.from | identity)
        and (.id | identity)
        and .from == $rejection.from and .id != $rejection.id
        and ($rejection.id | identity)
        and (.bind_declaration.resolves | index($rejection.id)) != null
      ) | not)
    | if (.id | type == "string" and length > 0) then .id else "unknown-router-rejection" end
  ] | unique | join(", ") | select(length > 0)
