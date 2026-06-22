# Data flow

Every outbound network call this binary can make, with its trigger and its
exact payload. If a call you observe isn't on this list, that's a bug.

## 1. `sibil preflight` (no flags)

- `GET https://monitor.cordee.ovh/health` — reachability check only.
- No payload, no auth, nothing persisted.

## 2. `sibil preflight --connect SIB-CONNECT-XXXX-XXXX-XXXX`

- Same `GET /health` as above, then:
- `POST https://monitor.cordee.ovh/api/v1/preflight/probe`
  Body: see [`docs/payload-examples/preflight_probe_request.http`](docs/payload-examples/preflight_probe_request.http).
  Source: `cmd/preflight.go::sendPreflightProbe`.
- Nothing is written to disk by this command. Temp state (if any) is
  cleaned up on exit, including on error (the function returns before
  creating anything persistent).

## 3. `sibil init` + persistent connect (`--activate SIB-ACTIVATE-XXXX`)

- Registers the server with the backend and starts the trial clock.
- Writes `sibil.json` locally (token + config), permissions `0600`.
- This is the first command that creates persistent local state.

## 4. `sibil tunnel --enable` / `sibil start`

- Opens one outbound WebSocket to the relay (`tunnel/tunnel.go`).
  Reconnects with exponential backoff (2s → 5min) on disconnect — this is
  the only retry/backoff loop in the codebase, and it's outbound-initiated,
  never listening for unsolicited inbound connections.
- The relay forwards opaque HTTP-shaped messages (method/path/headers/body)
  between the mobile app and the local server on `127.0.0.1`. The backend
  routes bytes; it does not parse or store them (doc comment at the top of
  `tunnel/tunnel.go`).
- Every relayed request must carry `Authorization: Bearer <CLI token>` or
  the local server rejects it (`server/auth.go`).

## 5. `sibil audit [--bundle]`

- Zero network calls. Pure local collection
  (`collect/audit_system.go`, `collect/services.go`, `collect/network.go`,
  `collect/docker.go`) written to `audit.json` / `audit.md` /
  `redaction_report.json` on disk.
- `--bundle` only runs `tar.gz` locally. Still zero network calls.

## 6. `sibil redeem --code SIB-SETUP-XXXX-XXXX-XXXX`

- `POST /api/v1/service-pass/redeem` — exchanges a one-shot claim code for
  a locally stored Ed25519-signed service pass (`.sibil-passes/`). Binds to
  `SHA-256(CLI token)` on first use.

## 7. `sibil setup-receipt`

- Reads local `sibil doctor` output and the local health picture to build
  a receipt file. Does not call the backend to generate the receipt
  content itself (the receipt documents what already ran locally).

## What never happens, anywhere in this codebase

- No background polling outside the two explicit always-on loops above
  (tunnel reconnect, local HTTP server) — both are things you started.
- No call reads `.env` files belonging to other processes.
- No call reads or transmits secret values (see `collect/redact.go` for
  the patterns we actively strip from anything that is collected).
- No telemetry/analytics beacon distinct from the product functionality
  itself — every network call above exists because you ran the command
  that triggers it.

Verify this yourself:
```bash
grep -rn '\.Get(\|\.Post(\|http\.NewRequest\|websocket\.Dial' --include="*.go" . | grep -v _test.go
```
That's the complete list of places this binary can talk to the network —
cross-check each line against the table above. `cmd/doctor.go`'s `.Get(`
call is the only one that targets `127.0.0.1` (your own local server's
`/health`), not the backend.
