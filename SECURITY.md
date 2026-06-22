# Security

## Verifying what you run

Every release ships `checksums.txt` alongside the binaries. Verify before
running anything, every time, including updates — and verify that
`checksums.txt` actually covers the exact file you downloaded, rather than
trusting `--ignore-missing` to tell you that on its own (it stays silent,
exit 0, if your file simply isn't listed):

```bash
FILE=sibil-linux-amd64
grep -q "  ${FILE}\$" checksums.txt && sha256sum -c <(grep "  ${FILE}\$" checksums.txt)
```

## Provenance attestations (optional, advanced)

Starting with `v1.4.1`, every release is built by GitHub Actions directly
from this repo's source (`.github/workflows/release.yml`) and the resulting
`checksums.txt` is attested — no binary is ever built or uploaded by hand.
If you have `gh` ≥ 2.49:

```bash
curl -LO https://github.com/sibil-monitor/sibil-agent/releases/download/v1.4.1/sibil-linux-amd64
curl -LO https://github.com/sibil-monitor/sibil-agent/releases/download/v1.4.1/checksums.txt

grep -q "  sibil-linux-amd64\$" checksums.txt && sha256sum -c <(grep "  sibil-linux-amd64\$" checksums.txt)

gh attestation verify ./sibil-linux-amd64 --repo sibil-monitor/sibil-agent
```

`gh attestation verify` checks a cryptographically signed statement (Sigstore)
tying the exact binary digest to: this repository, the `release.yml`
workflow, the commit it ran from, and the tag that triggered it. It does
**not** mean the binary is "secure" or free of bugs — it means you can
verify *where it came from and how it was built*, and decide for yourself
whether that's enough. The checksum step above remains the baseline; this
is an additional, optional layer on top of it.

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
