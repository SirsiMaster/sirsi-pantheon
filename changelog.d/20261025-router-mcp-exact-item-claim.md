Expose optional exact item id on router_claim through the existing ClaimItem
selector, preserving ClaimNext when omitted and the15-minute own-instance map.
Invalid supplied IDs/unknown keys refuse before RPC; exact refusal never claims
older or foreign work. No durable session-binding change or DC01 closure claim.

Normalize typed ErrNoWork only for omitted-ID ClaimNext to a friendly empty-inbox
reply without a session claim; exact unavailable items remain errors. Fixtures
use chronological ID prefixes matching the store’s ID ordering, so exact-target
selection is tested against an earlier eligible own item.
