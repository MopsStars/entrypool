#!/bin/sh
# Install entrypool (RU entry health checker).
# Prefer running the timer on a host that is not the only monitored entry.
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo sh install.sh" >&2
  exit 1
fi

for command in curl grep tar sha256sum systemctl; do
  if ! command -v "$command" >/dev/null 2>&1; then
    # tar/sha256sum only needed for release download; curl optional if building
    :
  fi
done

ENTRYPOOL_VERSION=${ENTRYPOOL_VERSION:-1.0.0}
REPO=${ENTRYPOOL_REPO:-}

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

SOURCE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

LOCAL_BINARY="$SOURCE_DIR/dist/entrypool-linux-$ARCH"
if [ -f "$LOCAL_BINARY" ]; then
  install -m 755 "$LOCAL_BINARY" /usr/local/bin/entrypool.new
elif command -v go >/dev/null 2>&1 && [ -f "$SOURCE_DIR/go.mod" ]; then
  (cd "$SOURCE_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
    go build -trimpath -ldflags="-s -w -X main.Version=${ENTRYPOOL_VERSION} -X main.Commit=local" \
    -o "$TMP_DIR/entrypool" .)
  install -m 755 "$TMP_DIR/entrypool" /usr/local/bin/entrypool.new
elif [ -n "$REPO" ] && command -v curl >/dev/null 2>&1; then
  ASSET="entrypool-linux-$ARCH"
  RELEASE_URL="https://github.com/${REPO}/releases/download/v${ENTRYPOOL_VERSION}"
  curl -fsSL --retry 3 "$RELEASE_URL/$ASSET" -o "$TMP_DIR/$ASSET"
  if curl -fsSL --retry 3 "$RELEASE_URL/SHA256SUMS" -o "$TMP_DIR/SHA256SUMS" 2>/dev/null; then
    grep " $ASSET\$" "$TMP_DIR/SHA256SUMS" > "$TMP_DIR/$ASSET.sha256"
    (cd "$TMP_DIR" && sha256sum -c "$ASSET.sha256")
  fi
  install -m 755 "$TMP_DIR/$ASSET" /usr/local/bin/entrypool.new
else
  echo "Need local dist/, go toolchain, or ENTRYPOOL_REPO=owner/repo + curl" >&2
  exit 1
fi
mv -f /usr/local/bin/entrypool.new /usr/local/bin/entrypool

mkdir -p /etc/entrypool /var/lib/entrypool
if [ ! -f /etc/entrypool/config.env ]; then
  entrypool init-config
  echo "Edit /etc/entrypool/config.env then:"
  echo "  systemctl enable --now entrypool.timer"
fi

install -m 644 "$SOURCE_DIR/systemd/entrypool.service" /etc/systemd/system/entrypool.service
install -m 644 "$SOURCE_DIR/systemd/entrypool.timer" /etc/systemd/system/entrypool.timer
systemctl daemon-reload

echo "Installed: $(entrypool version)"
echo "Config:    /etc/entrypool/config.env"
echo "Enable:    systemctl enable --now entrypool.timer"
echo "Once:      entrypool doctor && entrypool run"
