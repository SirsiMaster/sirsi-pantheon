# Relay spool discovery

The existing relay scans the durable spool every 150 ms. A single enumeration of each lane's request directory now supplies both request discovery and the existing warn-once permission diagnostic. Request files and `.json`-suffixed entries are handled by the existing worker and claim path; no scan result authorizes a request or proves completion. A spool pathname is treated literally rather than as a glob pattern.

This is a source-level reduction in duplicate directory reads. It retains polling and does not claim event-driven wakeup, production CPU reduction, installed runtime adoption or end-to-end latency parity. A separately qualified signed release and cooperative host-specific handoff remain necessary; do not restart or replace an active relay based only on source evidence.
