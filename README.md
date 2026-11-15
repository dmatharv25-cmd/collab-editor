[![CI](https://github.com/dmatharv25-cmd/collab-editor/actions/workflows/ci.yml/badge.svg)](https://github.com/dmatharv25-cmd/collab-editor/actions/workflows/ci.yml)

# collab-editor

A real-time collaborative text editor with a **hand-built CRDT**, written in Go and TypeScript.

**[Live demo](https://collab-editor-lfi1.onrender.com/?room=try-me)**: open it in two tabs and type. The demo runs on a free instance, so it can take up to a minute to wake up, and it runs in memory-only mode, so documents reset when it restarts. It is public, so don't type anything private. For a private room, change `room=` in the URL to any name you like.
Edit the same document from multiple browser tabs, go offline, keep typing, and everything merges on reconnect.

![Demo: two windows editing together, one goes offline and merges back](docs/demo.gif)

## Features

- **RGA CRDT written from scratch** in Go and TypeScript, with tombstones and Lamport-style IDs
- **Conflict-free merging**: concurrent edits converge on every replica, whatever order they arrive in
- **Offline editing**: edits are queued locally and merged on reconnect, with automatic retry and backoff
- **Go WebSocket server** with per-document rooms and full history replay for late joiners
- **Cross-language conformance**: CI regenerates test vectors from the Go code and checks that the TypeScript CRDT produces identical documents

## Performance

Replacing a slice and linear scans with a linked list, an ID index and a cursor "finger" made appending 10,000 characters **36x faster** (125.9 ms to 3.54 ms). Details in [docs/BENCHMARKS.md](docs/BENCHMARKS.md).

## Run it locally

Terminal 1: `cd server`, then `go run .`

Terminal 2: `cd client`, then `npm install`, then `npm run dev`

Open `http://localhost:5173/?room=demo` in two tabs.

## Tests

`cd server && go test ./... -race` and `cd client && npm test`

## Roadmap

- [x] Acknowledgements and resend for ops lost in flight
- [x] Persistence (operation log in a database)
- [x] Load test with 50+ simulated users (p95 latency)
- [ ] Tombstone compaction

## Load test

`loadtest/loadtest.py` opens N websocket clients on a fresh room, sends inserts at a fixed rate, and measures send-to-ack latency. Because the server saves each op to SQLite before acking, the latency includes the disk write. At the end it checks that the server's text length equals clients x ops.

Results on one Windows machine (server and Python clients on the same box, so client CPU is included). Every run: 0 ops lost, all rooms converged.

| Load | Mode | p50 | p95 | p99 |
|---|---|---|---|---|
| 50 clients, ~100 ops/s, 1 room | SQLite | 23.5 ms | 57.7 ms | 96.5 ms |
| 50 clients, ~100 ops/s, 1 room | memory only | 16.4 ms | 38.4 ms | 45.4 ms |
| 200 clients, ~760 ops/s, 4 rooms | SQLite | ~39 ms | ~130 ms | ~175 ms |
| 200 clients, ~760 ops/s, 4 rooms | memory only | ~20 ms | ~55 ms | ~92 ms |

Saving before acking costs about 7 ms at p50 for a single room and about 19 ms at p50 under 4 busy rooms. All writes share one SQLite connection; the next section shows how batching them fixes this.

To run it, start the server and then:

    pip install websockets
    python loadtest/loadtest.py --clients 50 --ops 100 --rate 2

To load several rooms at once, start more than one process; each picks its own room name.

### Batched SQLite writes

Writes from all rooms are grouped into one transaction per disk write (group commit), and the room lock is released while the write happens. An ack still means the op is on disk. Same test, 200 clients, about 760 ops/s, 4 rooms, 0 ops lost:

| Mode | p50 | p95 | p99 |
|---|---|---|---|
| SQLite, one write per op | ~39 ms | ~130 ms | ~175 ms |
| SQLite, batched | ~20 ms | ~45 ms | ~58 ms |
| memory only | ~20 ms | ~55 ms | ~92 ms |

## How it works

- **CRDT (RGA):** every character is an op with a unique ID (client + Lamport clock) and a pointer to the character it follows. Concurrent inserts are ordered by a deterministic tie-break, so all replicas converge to the same text regardless of arrival order. The Go server and the TypeScript client share test vectors (`conformance/`) to prove both implementations agree.
- **Offline editing:** the client keeps working while disconnected, queues its ops, and merges on reconnect.
- **Acks and resend:** each op has a key. The server acks after the op is saved; the client resends anything unacked after a reconnect. The server de-duplicates by key, so resends are safe.
- **Persistence:** every op is appended to SQLite before it is acked, so an ack means the op is on disk. On startup a room is rebuilt by replaying its ops, including ones that arrive before their dependencies.
- **Group commit:** a single writer goroutine batches writes from all rooms into one transaction, and the room lock is released during the disk write. This cut p95 ack latency under load from about 130 ms to about 45 ms.
- **Load test:** `loadtest/loadtest.py` drives many websocket clients and verifies convergence.

Known limits: the op log grows without bound (no snapshots or compaction yet), and there is no auth.
