# Privacy

This document describes exactly what data can leave your server at each
stage, and is meant to be checked against the code, not trusted on its own.
Every "yes" in this table maps to a specific function — linked below.

## What moves, by surface and by stage

| Surface | Preflight | Trial | Paid monitoring | Audit expert |
|---|---|---|---|---|
| OS / arch | yes | yes | yes | yes |
| PM2 / systemd presence (boolean) | yes | yes | yes | yes |
| Service **names** | no | yes | yes | yes |
| Ports / listeners | no | depends on plan | yes | yes |
| Raw logs | no | no by default | no by default | only if consented |
| Metrics (CPU/RAM/disk) | no | yes | yes | yes |
| Secrets | never | never | never | never |
| `.env` files | never | never | never | never |
| Service control actions | no | no | Pro + opt-in only | per human-led engagement |

"Never" rows are not a policy promise — they're a structural fact: this CLI
has no code path that reads `.env` file contents or secret values and
transmits them. `collect/redact.go` additionally strips common secret
patterns from anything that does pass through logs/audit output, as a
second layer.

## Per-stage detail

### Preflight (`sibil preflight`)
Ephemeral, no `--connect`: nothing leaves the machine, only a local report
and a `GET /health` reachability check.
With `--connect CODE`: exactly one POST, shown byte-for-byte in
[`docs/payload-examples/preflight_probe_request.http`](docs/payload-examples/preflight_probe_request.http).
No service names, no ports, no logs, no inventory. Source:
`cmd/preflight.go::sendPreflightProbe`.

### Trial / paid monitoring
Once you persist a connection (`sibil init` + tunnel, or `--activate`), the
mobile app can read whatever your entitlement allows: service list, metrics,
optionally logs. This is relayed through the backend as opaque bytes over
the tunnel — the backend does not parse or store this traffic (see
`tunnel/tunnel.go` doc comment). Module **labels** you choose to display are
stored backend-side as display configuration, not as infrastructure data.

### Audit (`sibil audit`)
Everything is written to local files (`audit.json`, `audit.md`,
`redaction_report.json`). There is no upload code path in this repo —
search it yourself: `grep -rn "http.Post\|http.Client" cmd/audit.go`. If you
choose to share the resulting file with a human auditor, that's a decision
you make outside this tool, not something the CLI does for you.

### Service control actions
Disabled by default. Requires both a local opt-in (`sibil actions enable`)
**and** a compatible signed entitlement (`can_control_services: true`).
Either one missing fails closed (`server/actions.go`).

## What we will not promise (see `do_not_sell_yet` doctrine)

- certified/critical monitoring or an SLA
- end-to-end encryption above the relay (the relay is TLS-transported,
  not E2E-encrypted at the application layer)
- automatic generation of an architecture audit from CLI signals alone —
  `sibil audit` produces signals; turning them into a verdict is a human,
  paid step
