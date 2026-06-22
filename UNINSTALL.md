# Uninstall

```bash
sibil uninstall          # dry run — shows what would be removed, removes nothing
sibil uninstall --yes    # actually removes it
```

Nothing is deleted without `--yes`. This is enforced in code
(`cmd/uninstall.go::runUninstall`), not just documented.

## What gets removed

- the `sibil` binary
- the `sibil` PM2 process, if detected
- the `sibil` systemd unit, if detected
- `sibil.json` (config + token)
- `.sibil-passes/` (locally stored service passes)
- `~/.sibil/` (manifest + any cached data)

## What is never touched

- your other PM2 applications
- your other systemd services
- your logs, databases, firewall rules — anything that isn't Sibil's own
  state

`uninstall` discovers what it removes from two sources: the local manifest
written at install/init time (`config/manifest.go`), and a live scan (PM2
`jlist`, `systemctl is-active`) so a manually moved install still gets
cleaned up. If something Sibil-owned exists outside the documented list
above and `--yes` doesn't remove it, that's a bug — open an issue.

## Flags

- `--path <dir>` — directory where Sibil was initialised, if not the
  current directory
- `--config <path>` — explicit path to `sibil.json`

## If you're uninstalling because something went wrong

`sibil doctor` first, to capture what state things were in — it's
read-only and won't interfere with the uninstall that follows.
