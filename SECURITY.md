# Security

## Verifying what you run

Every release ships `checksums.txt` alongside the binaries. Verify before
running anything, every time, including updates:

```bash
sha256sum -c checksums.txt --ignore-missing
```

We're moving towards GitHub artifact attestations (provenance: which commit
in this public repo produced this exact binary) — track progress in
[releases](https://github.com/sibil-monitor/sibil-agent/releases). Once
live, verify with:

```bash
gh attestation verify sibil-linux-amd64 --owner sibil-monitor
```

## Cryptographic trust model

- **Entitlement tokens**: Ed25519, signed backend-side, verified here with
  a public key embedded in source (`server/entitlement.go`,
  `entitlementPublicKeys`, kid `sibil-entitlement-v1`). 24h TTL + 72h grace
  window for clock skew / transient backend issues. No network call is
  needed to verify a token you already have.
- **Service passes**: same scheme, different keyspace (`server/service_pass.go`,
  kid `sibil-service-pass-v1`). Bound to a specific agent via
  `SHA-256(CLI token)` on first redeem, so a leaked claim code can't be
  reused against a different machine after that.
- This repo never contains a private signing key. If you ever find one in
  a commit, that's a critical bug — see "Reporting a vulnerability" below.

## Local server

- Binds `127.0.0.1` only (`server/server.go`). It is not reachable from the
  network unless you put something in front of it yourself — don't.
- Every relayed request carries `Authorization: Bearer <CLI token>`
  (`server/auth.go`); requests without it are rejected, not silently
  downgraded.
- Service control actions (start/stop/restart) require both a local opt-in
  flag and a compatible entitlement — removing either one disables them
  immediately (`server/actions.go`).
- `sibil doctor` is intentionally read-only by design (see the comment in
  `cmd/doctor.go`) — it never repairs anything automatically.

## Redaction

`collect/redact.go` strips common secret patterns (API keys, Stripe keys,
`Authorization` headers, JSON `password`/`token` fields, common env-var
secret names) from any text that passes through audit/log collection. It's
a second layer, not the only one — the first layer is simply: this CLI
doesn't read `.env` file contents or process environment variables of other
services in the first place.

## Threat model, briefly

In scope:
- A malicious actor with network access trying to impersonate the backend
  or replay an entitlement token.
- A leaked claim/connect code being reused beyond its intended scope.
- Secrets accidentally leaking through collected service logs.

Out of scope for this CLI alone:
- Compromise of the host OS itself (this is a monitoring agent, not a
  hardening tool).
- Anything happening after you explicitly enable destructive actions on a
  compatible plan — at that point you've opted into remote control, by
  design, and the blast radius is yours to manage.

## Reporting a vulnerability

Open a private security advisory on this repo
(`Security` tab → `Report a vulnerability`), or email the address listed in
the repo's GitHub profile. Do not open a public issue for anything that
could be exploited before a fix ships. We'll acknowledge within a few days.
