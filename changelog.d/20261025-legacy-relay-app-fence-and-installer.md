The optional legacy root-relay maintenance app path now binds each reviewed
operation to a controller-issued durable ticket and monotonic generation under
the existing service lock. Explicit resolution records a matching result or
fences delayed arrival before offering fresh consent. Old or empty status never
clears a newer unknown receipt. Cancellation preserves uncertain outcomes;
recovery, abort and replacement restoration require their own captured tickets.

Protected-engine preflight is bounded and checks the installed helper's native
protocol/release against the signed app before authorization. A reachable app
action prepares and rechecks the exact running release's normal PKG, including
published SHA256, package trust, replacement metadata, payload and helper
compatibility, before opening macOS Installer. Installation and fresh installed
pair verification remain separate; a DMG handoff or an older incompatible PKG
is not substituted.

This is an unqualified source successor to the retained 54d bridge review and
failed fixture-compile attempt. Authored inert fixtures are not execution or
installed-recovery evidence. Existing modern service/agent authorization and
producer-covered Studio maintenance remain unchanged; this fallback adds no
Studio rollout gate or automatic service action.
