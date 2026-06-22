# Architecture

## Three blocks, one boundary

```
┌─────────────────────┐        ┌──────────────────────┐        ┌─────────────────┐
│   Mobile app         │  WS/HTTPS │  Backend (closed)   │  WS  │  This repo       │
│   (closed)            │◄────────►│  auth, billing,       │◄────►│  sibil-agent      │
│                        │          │  entitlement signing, │      │  runs on YOUR VPS │
│                        │          │  tunnel relay         │      │  127.0.0.1 only   │
└─────────────────────┘        └──────────────────────┘        └─────────────────┘
                                  no infra data stored                collects, redacts,
                                  here (see PRIVACY.md)                serves locally
```

This repo is the rightmost box. It is the only piece of Sibil that ever
touches your filesystem, your process list, or your network listeners.

## What's in this repo

| Path | Role |
|---|---|
| `cmd/` | CLI commands (`init`, `start`, `doctor`, `preflight`, `tunnel`, `audit`, `uninstall`, …) |
| `collect/` | Local collectors: system, services (PM2/systemd/Docker), network listeners, redaction |
| `config/` | `sibil.json` load/save, on-disk manifest of what was installed (used by `uninstall`) |
| `server/` | The local read-only HTTP API (`127.0.0.1` only), entitlement + service-pass **verification** |
| `tunnel/` | WebSocket client that relays HTTP requests from the mobile app to the local server |

## What's not in this repo, and why

| Component | Why it's closed |
|---|---|
| Dashboard / mobile app UI | Product surface, not infrastructure access |
| Billing / Stripe integration | Payment processing, no local data involved |
| Backend (auth, subscription, entitlement **signing**) | Holds private keys and customer data; this repo only ever holds the matching **public** key to verify, never to sign |
| Audit/report generation (human side) | `sibil audit` collects facts locally (see `cmd/audit.go`); turning facts into a prioritized report is a paid human service, not a CLI feature |
| Admin tooling | Internal operations surface |

The rule we hold ourselves to: **anything that touches your machine is in
this repo.** Anything that's a SaaS product decision (billing, dashboards,
premium analysis) lives elsewhere and never needs to.

## Trust boundary: entitlement verification

The backend signs a short-lived entitlement token (Ed25519) describing what
plan you're on (`can_view_system`, `can_view_logs`, `can_control_services`,
module limits). This repo embeds only the **public** key
(`server/entitlement.go`, `entitlementPublicKeys`) and verifies signatures —
it cannot mint its own entitlements. Same pattern for service passes
(`server/service_pass.go`).

If you remove network access entirely, the local server falls back to
`degradedClaims()`: one module, no logs, no actions. It fails closed, not
open.

## Local server

`sibil start` binds `127.0.0.1` only — never `0.0.0.0`. The only way data
leaves the machine is:
1. A request relayed through the tunnel (`tunnel/tunnel.go`), authenticated
   per-request with `Authorization: Bearer <CLI token>`.
2. A direct HTTP request to a URL you've explicitly exposed yourself (not
   something this CLI does on its own).
3. The two opt-in calls described in [DATA_FLOW.md](DATA_FLOW.md)
   (`preflight --connect`, `connect-persistent`).

There is no background telemetry beacon. No code path in this repo opens an
outbound connection except the ones listed above.
