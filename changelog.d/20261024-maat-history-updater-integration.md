- Fence both native Ma'at history loaders by refresh generation. Delayed or
  cancelled responses cannot replace a newer page or query; current failures
  retain the latest committed projection and its matching continuation.
- Keep Ma'at history filters ahead of bounded page selection, preserve explicit
  empty-array and query-versus-after-cursor provenance, and continue exact native
  action reads only across the same query and snapshot. Owner acceptance can
  locate an old exact review through a separate bounded strict journal scan;
  tolerant history cursors never authorize mutation.
- Retain exact native read progress across explicitly continued interactions,
  bind it to the selected action generation, and keep raw evidence multiplicity
  separate from hidden relationship rows. Charge selected-record rereads and
  limit finalization to the remaining budget or one bounded dependency bundle;
  empty, incomplete, after-cursor and failed-refresh states remain distinct.
- Project binary updater outcomes and unresolved attempts through the same
  Casebook overlay before history status/text filtering and page selection.
  Preserve binary-only repair success separately from other findings and failed
  floors, and retain typed inspect/re-screen routes and receipt search.
- Owner acceptance keeps its strict journal mutation path. Source fixtures and
  static inspection do not qualify installed behavior or execute an updater.
