Periodic native memory reads now use `vitals --summary`, preserving capacity,
pressure and swap while avoiding process ranking and emitting `top: []`.
Explicit CLI vitals retains ranked evidence. The browser Host view offers an
on-demand Inspect memory read using the same ranked producer, with truthful
empty, retained/error and retry states, including when router data is unavailable.
