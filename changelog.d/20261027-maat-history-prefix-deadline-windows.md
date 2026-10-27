Ma'at history prefix verification now rejects physical EOF before the anchored
boundary, even if the inode is restored before draining. A verification
deadline with no verified cursor progress returns an error instead of an
unchanged continuation. Windows keeps the prior bounded reader, including its
append and cancellation semantics; frozen-prefix verification remains
Unix-scoped.
