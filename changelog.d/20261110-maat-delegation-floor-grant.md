- **feat(maat):** a booking on a resource delegated to a pod authority now gets
  the bounded floor share instead of a hard refusal, so a lane is never locked
  out of a machine. Quiet requests, `--queue`, requests inside the handback
  margin, and windows protected by a replayed authority quiet/loaded hold are
  still refused with "book through the pod authority". The canonical ledger
  cannot see authority holds that have not been replayed to it.
