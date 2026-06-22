# sibil-agent

> Everything that runs on your server is open source. The managed cloud is paid.

This repo is the CLI/agent that runs **on your VPS**. It collects local
facts (system, services, network listeners), redacts secrets, and serves
them to the Sibil Monitor mobile app — either directly or through an
outbound-only WebSocket tunnel. It never phones home on its own, and it
never uploads anything unless you run a command that explicitly does so.

What's closed source and why: see [ARCHITECTURE.md](ARCHITECTURE.md#whats-not-in-this-repo).

## Install — recommended path

Download, verify, run. Nothing is installed by this first step.

```bash
curl -LO https://github.com/sibil-monitor/sibil-agent/releases/download/v1.4.0/sibil-linux-amd64
curl -LO https://github.com/sibil-monitor/sibil-agent/releases/download/v1.4.0/checksums.txt
sha256sum -c checksums.txt --ignore-missing

chmod +x ./sibil-linux-amd64
./sibil-linux-amd64 preflight
```

`preflight` is read-only and ephemeral: it checks OS/arch/PM2/systemd/Docker
and outbound reachability, prints a compatibility report, and creates no
files. See [DATA_FLOW.md](DATA_FLOW.md) for exactly what it does and does
not send.

Once you're satisfied, install it:

```bash
sudo install -m 755 sibil-linux-amd64 /usr/local/bin/sibil
sibil init
sibil doctor
sibil start
```

## Install — advanced (inspectable script)

If you'd rather not run three commands by hand, read the script first,
then run it:

```bash
curl -fsSL https://github.com/sibil-monitor/sibil-agent/releases/latest/download/install.sh -o sibil-install.sh
less sibil-install.sh
sh sibil-install.sh
```

It does exactly what the manual path above does: download, verify
checksum, install. See [install.sh](install.sh).

## Install — fast path (least recommended)

```bash
curl -fsSL https://monitor.cordee.ovh/bootstrap | sh
```

This runs `sibil preflight` in a temp dir and deletes everything on exit.
Convenient for a first look, not how we'd install on a server we care about.
(`monitor.cordee.ovh` is migrating to `sibil.sh` — this command will move
with it; the old domain will keep working during the transition.)

## Commands

| Command | What it does |
|---|---|
| `sibil preflight [--connect CODE]` | ephemeral compatibility check, no install |
| `sibil init` | generate `sibil.json` config + local token |
| `sibil doctor` | read-only health check of your local setup |
| `sibil start` | start the local read-only API server (binds `127.0.0.1` only) |
| `sibil tunnel --enable` | open the outbound WebSocket relay to the mobile app |
| `sibil actions enable` | opt in to service control (start/stop/restart), gated by entitlement |
| `sibil audit [--bundle]` | local 4-layer observability snapshot, written to disk, never uploaded |
| `sibil setup-receipt` | generate a receipt for a completed Sibil Setup session |
| `sibil redeem --code CODE` | redeem a one-shot service pass (setup/audit) |
| `sibil passes` | list locally stored service passes |
| `sibil uninstall [--yes]` | dry run by default; removes binary, PM2/systemd unit, config, passes |

Full command reference: `sibil --help` / `sibil <command> --help`.

## Docs

- [ARCHITECTURE.md](ARCHITECTURE.md) — what runs where, what's open vs. closed
- [PRIVACY.md](PRIVACY.md) — exactly what data moves at each plan tier
- [SECURITY.md](SECURITY.md) — entitlement verification, redaction, threat model, vuln reporting
- [DATA_FLOW.md](DATA_FLOW.md) — every network call this binary makes, byte for byte
- [UNINSTALL.md](UNINSTALL.md) — what gets removed and what never does
- [docs/tcpdump/](docs/tcpdump/) — verify network behavior yourself
- [docs/payload-examples/](docs/payload-examples/) — real request/response bodies

## Building from source

```bash
go build -o sibil .
go test ./...
```

Go 1.22+, no CGO, no external runtime dependencies.

## License

MIT — see [LICENSE](LICENSE).
