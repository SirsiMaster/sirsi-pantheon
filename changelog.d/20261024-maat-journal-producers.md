Fix current Ma'at journal producer compatibility: conflict-check attributes the
calling SIRSI_AGENT_ID, or explicitly the command producer when unspecified,
while retaining the reservation holder as affected. Checker errors remain
unverified. Cede and floor projections serialize canonical time, not ts, without
changing actors or post-commit projection error reporting.

Owned temporary-journal fixtures use FileDecisionJournal.Recent as the strict
oracle for no-holder, checker-error, cede and floor projections. Qualification
is unrun in this source checkpoint. No historical records are changed, no
loaded writer is attributed, and no installed repair or UI completion is claimed.
