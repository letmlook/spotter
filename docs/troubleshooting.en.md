# Troubleshooting

Symptom-first guide. Each section lists the cause in order of likelihood
based on the kinds of bugs we've already seen in 0.1.x development.

## 1. Devices never show up in the client

Most common cause: **agent and client are on different L2 broadcast
domains.** UDP multicast does not traverse routers unless IGMP/PIM is
explicitly set up.

How to verify:

```bash
# On the device:
ip addr show | grep inet          # note the IPv4 subnet
journalctl -u spotterd --no-pager | tail -n 20

# On the operator's machine (any OS that has tcpdump / nping):
tcpdump -n -i <iface> 'udp port 9999'    # watch 60 s for HELLO packets
```

If `tcpdump` sees nothing on the operator side but `journalctl` shows the
agent broadcasting every 60 s, **a router or firewall is silently
dropping multicast between them.** Fix the network — Spotter cannot.

Workarounds while you fix the network:

- Use **Add Device by IP** in the GUI to register the device manually.
- Or trigger a manual subnet scan from **Tools → Scan Subnet**.

## 2. Device shows up, then disappears

Every 30 s the client polls every entry's `/api/v1/info`. If the poll
fails 3 times in a row the row flips to offline. Common causes:

- **Spotterd restart** — the client catches up on the next poll cycle.
  No action needed.
- **Network blip** — same as above; offline status goes green within
  ~90 s once the network returns.
- **`listen_addr` is wrong on the device** — `journalctl -u spotterd`
  will show `bind: address already in use` or `bind: cannot assign
  requested address` if the configured interface is gone. Edit
  `/etc/spotterd/agent.toml` and `systemctl restart spotterd`.
- **TTL expiry** — long-lived UDP cache entries in some consumer routers.
  `nmap -sU -p 9999 --ttl 1 <device-ip>` should NOT show the listening
  port; the TTL=1 mcast loop test we ship doesn't help here. Instead
  work around by switching to subnet scan.

## 3. Subnet scan finds nothing

The scanner auto-detects the operator's local subnet. If you have
multiple NICs (Wi-Fi + Ethernet, or VPN + LAN), it picks the first
RFC1918 one. A manual override is available:

- **Tools → Scan Subnet → Specify CIDR** → e.g. `192.168.1.0/24`.
- VPN subnets typically live outside RFC1918; supply the CIDR.

TCP probe uses Go's `net.DialTimeout` with a 500 ms timeout per IP. On
flaky Wi-Fi you may want to re-run the scan twice — the second run picks
up devices the first missed.

## 4. install.sh hangs on sudo

> `sudo bash /tmp/install.sh` then nothing.

`sudo` is asking for a password on the TTY. SSH non-interactive doesn't
allocate one by default. Fixes, in order of preference:

1. Configure passwordless `sudo` for the deploy user (recommended).
2. `ssh -t user@device` to force a TTY allocation, then run the script.
3. Append ` NOPASSWD: ALL` for the deploy user under
   `/etc/sudoers.d/`.

`scripts/deploy.sh` uses **public-key mode** when no password is given;
that's the most reliable path.

## 5. systemctl reports "Unit spotterd.service not found"

`spotterd.service` was not copied to `/etc/systemd/system/`. Re-run the
deployment script. If you scp'd manually:

```bash
sudo install -m 0644 /tmp/spotterd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now spotterd
```

## 6. UI is blank or stuck on "Loading…"

Likely a JS exception during startup. The client logs go to
`<UserConfig>/Spotter/logs/spotter.log`. If the file is empty, the
issue is below the slog handler; on macOS also check
`~/Library/Logs/Spotter/` for OS-level crash reports.

Quick diagnostic:

```bash
# macOS
open -a Console.app && # filter to "Spotter"
# Linux
journalctl --user -u spotter-client.service || tail -n 200 ~/.config/Spotter/logs/spotter.log
```

Common causes:

- **Stale `frontend/dist`** — wipe `build/bin/Spotter.app` and run
  `make client` again.
- **Mixed-protocol `device_id`** — only happens if `agent_version` on
  the device is much newer than the client; update the client.

## 7. "registry cleared" didn't take effect

`Tools → Clear Registry` removes every entry from the local JSON but the
**scanner still has devices in memory**. Reload the GUI by quitting and
reopening — there is no hot-reload path for the registry in 0.1.0.

## 8. schema_version mismatch

If the agent says `schema_version: 2` but the client only knows
`schema_version: 1`, the GUI logs `decode: schema version 2 not supported`.

Fix: align the agent_version with the client's pinned version. There is
no automatic downgrade — the protocol design assumes rolling forward.

## 9. Auto-discovery works, but "Add by IP" returns probe: HTTP 4xx

`Add Device by IP` issues a real `GET /api/v1/info`. If the device
returns 401/403, the agent is misconfigured (this is expected to be
impossible in 0.1.0; it would only happen if a future version added
authentication). Otherwise:

- Wrong port — `spotterd` defaults to 9999. The client fills that in;
  supply a different port only if you customised `/etc/spotterd/agent.toml`.
- Wrong IP / firewall on the device side — verify with `curl http://<ip>:9999/api/v1/info`
  from the operator's box.

## 10. How to file a useful bug report

If none of the above matches, file an issue using the Bug Report
template (`.github/ISSUE_TEMPLATE/bug_report.yml`). Include:

- Versions: `uname -a`, `cat /etc/os-release`, agent `agent_version`,
  client Help → About string.
- Device IP(s) and operator host IP(s), and whether they're on the same
  L2 domain.
- `journalctl -u spotterd --no-pager -n 200` from the device, plus the
  client's `spotter.log`.

Security issues do **not** go in the public tracker — see
`SECURITY.md`.
