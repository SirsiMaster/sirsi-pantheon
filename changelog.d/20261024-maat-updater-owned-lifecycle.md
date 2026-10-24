The confirmed Ma'at binary-drift updater is now bounded to two minutes, runs in
an owned process group, and handles cancellation by stopping only its own
children before recording a non-resolved terminal outcome. Native Casebook
shows an in-progress state and keeps retry behind terminal completion and fresh
review. No live updater was run for this source change.
