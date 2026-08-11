# entrypool

RU entry health checker for multi-IP `entry.mops.network` (kuzrelay-style CLI).

Probes TCP ports on each RU entry, then:

| MODE | Behavior |
|---|---|
| `pool` | Keep Remnawave Hosts on `ENTRY_NAME`; sync **Cloudflare A** to healthy IPs only |
| `failover` | Legacy: Remnawave `address` → live IP if only one entry is up |
| `alert` | Probe + Telegram only (no DNS/panel writes) |

Works with **N** entries (`ENTRY_IPS=ip1,ip2,ip3`), not just two.

## Install

```bash
git clone https://github.com/MopsStars/entrypool.git
cd entrypool
sudo sh install.sh
sudo nano /etc/entrypool/config.env
sudo systemctl enable --now entrypool.timer
```

Or from a release binary (after you publish):

```bash
# install.sh downloads entrypool-linux-amd64 from GitHub Releases
ENTRYPOOL_VERSION=1.0.0 sudo sh install.sh
```

## Commands

```bash
entrypool doctor      # probe now
entrypool run         # one cycle (timer)
entrypool status      # last state
entrypool version
```

## Config (`/etc/entrypool/config.env`)

```bash
MODE=pool
ENTRY_NAME=entry.mops.network
ENTRY_IPS=1.2.3.4,5.6.7.8,9.10.11.12
PROVIDERS=cloudflare

# Until Cloudflare NS: MODE=alert (manual DNS) or PROVIDERS= empty
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
```

Without Cloudflare, use `MODE=alert` and edit Orderbox A records by hand when Telegram fires.

## Build release assets

```bash
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w -X main.Version=1.0.0' -o dist/entrypool-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w -X main.Version=1.0.0' -o dist/entrypool-linux-arm64 .
(cd dist && sha256sum entrypool-linux-* > SHA256SUMS)
```

Upload `dist/*` to GitHub Release `v1.0.0`.

## Notes

- Run the timer on Beget / EU / a spare host — not only on one RU you are monitoring.
- Disable old Python `entry-dns-failover` if both would fight over Remnawave Hosts.
- `MODE=pool` needs zone NS on Cloudflare for automatic multi-A.
