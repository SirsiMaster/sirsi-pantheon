#!/bin/bash
set -euo pipefail
exec /usr/bin/python3 -B "$(cd "$(dirname "$0")" && pwd)/qualify-hosted-commercial-lifecycle.py"
