# Verifying network behavior yourself

Don't trust this document. Run the capture yourself — that's the point.

```bash
# Terminal 1 — capture outbound traffic from the sibil process
sudo tcpdump -i any -n host monitor.cordee.ovh -w preflight.pcap

# Terminal 2 — run the probe
./sibil preflight --connect SIB-CONNECT-XXXX-XXXX-XXXX

# Then inspect
tcpdump -r preflight.pcap -n
```

Traffic is TLS, so `tcpdump` shows you connection metadata (source/dest IP,
port, byte counts, TLS handshake) but not application data. To see the
actual bytes, decrypt at the application layer instead — that's what
[`preflight_capture_annotated.txt`](preflight_capture_annotated.txt) does,
by running the binary with `SIBIL_DEBUG_HTTP=0` against a local proxy
(e.g. `mitmproxy`) pointed at `monitor.cordee.ovh`, or by reading the
single call site directly: [`cmd/preflight.go`](../../cmd/preflight.go)
function `sendPreflightProbe`.

## What to expect

- One TCP connection, one TLS handshake, one HTTPS request, one response.
- No connection is opened anywhere except `monitor.cordee.ovh:443`
  (or your tunnel relay once Phase 4 domains are live).
- No background polling, no telemetry beacon, no second process.
- Closing the CLI closes the connection. There is no persistent agent
  unless you explicitly run `sibil start` (local server) or
  `sibil tunnel --enable` (outbound relay).

## Annotated example capture

See [`preflight_capture_annotated.txt`](preflight_capture_annotated.txt)
for a worked-through example with commentary on every line. It is a
documentation aid, not a substitute for running your own capture.
