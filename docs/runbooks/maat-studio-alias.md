# Studio capacity compatibility

The bare machine resources `studio` and `m5-studio` share reservation admission,
conflict detection, status, coverage, cede lookup and configured capacity.
Existing tickets retain their resource spelling and identity; use their existing
IDs for heartbeat, extend and release. Other resource names, rails and components
retain literal semantics. Executor admission still requires actual host binding.

If old capacity settings disagree, the smaller positive core/RAM setting applies
for either alias. Setting capacity through either name explicitly sets both
aliases to the requested value. This does not rewrite reservation history.

Qualification requires exact-head independent review and CI, then coordinated
canonical service/client release. Mixed server versions still permit shadow
pools; source tests alone do not establish deployed repair. No change to115 or
installation is authorized by this source packet. Native host alias binding is
outside this compatibility rule and must not be bypassed.
