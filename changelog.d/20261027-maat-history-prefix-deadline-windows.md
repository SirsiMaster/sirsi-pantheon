Ma'at history prefix verification now establishes its anchor before query
work, returns only resumable verified cursors when the remaining request
budget is insufficient, and keeps the response snapshot bound to that prefix.
Unix leaf opens are nonblocking before type checks. Windows retains the prior
bounded history reader and cursor format; frozen-prefix verification remains
Unix-scoped.
