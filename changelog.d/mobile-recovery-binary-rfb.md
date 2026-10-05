# Fix

- Send RFB bytes as binary WebSocket frames for noVNC; wire regression checks opcode 2 and preserves non-UTF-8 bytes in both directions.
