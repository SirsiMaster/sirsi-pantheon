Page diagnostic restart discovery across both request and historical-record
collections with explicit coverage, snapshot/query-bound continuation and
changed-read restart. Preserve strict whole-store/WAL validation and exact UUID
lookup; do not truncate or treat the first empty request page as complete.

Finish exact retained not-submitted result WAL before absent-journal inference,
then fence normally without observation or append. Reject explicitly empty
request selectors before they can fall through to legacy confirmed recording;
omitted selectors retain legacy behavior.
