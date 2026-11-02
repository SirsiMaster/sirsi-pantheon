# Exact reservation readback

Use `sirsi maat status --id <full-reservation-id> --json` for one canonical reservation, including RELEASED history. The selector is the full ID, never a prefix. Do not combine it with a resource argument. Existing `status [resource]` behavior stays compatible.

The existing MaatRead RPC uses the additive `reservation` kind. Deploy the compatible server before selecting the client command; an old server refuses and the client never falls back to a whole-history dump. Output is an array of zero or one rows, bounded to a 64KiB JSON result payload; unknown returns empty (not RELEASED), duplicate IDs and oversized records error without truncation. Exact text mode also skips unrelated cede reads. Existing authentication and lane scope are unchanged. Unsupported delegated pod IDs refuse canonical fallback; this is not a new pod-authority route.

The backend still reads its existing reservation state representation internally; this change bounds the selected response, not server-side historical storage or admission complexity. No reservation/admission mutation or additional store is introduced. Readback tests compare retained raw state before/after.

The reported installed142 operator capture ENOBUFS at8MiB is a read-buffer failure, not reservation loss. Native805f/0946 RELEASED evidence remains authoritative. This source is not a rollback request, installed upgrade or live workload action.
