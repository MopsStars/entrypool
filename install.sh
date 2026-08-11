#!/bin/sh
# Install entrypool from this repo (or GitHub release binary).
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo sh install.sh" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi
if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl is required" >&2
  exit 1
fi

ENTRYPOOL_VERSION=${ENTRYPOOL_VERSION:-1.0.0}
REPO=${ENTRYPOOL_REPO:-MopsStars/entrypool}

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

ASSET="entrypool-linux-$ARCH"
LOCAL_BINARY="$SOURCE_DIR/dist/$ASSET"
INSTALLED=0

if [ -f "$LOCAL_BINARY" ]; then
  echo "Installing local $LOCAL_BINARY"
  install -m 755 "$LOCAL_BINARY" /usr/local/bin/entrypool.new
  INSTALLED=1
fi

if [ "$INSTALLED" -eq 0 ]; then
  RELEASE_URL="https://github.com/${REPO}/releases/download/v${ENTRYPOOL_VERSION}"
  echo "Downloading $RELEASE_URL/$ASSET ..."
  if curl -fsSL --retry 3 "$RELEASE_URL/$ASSET" -o "$TMP_DIR/$ASSET"; then
    if command -v sha256sum >/dev/null 2>&1 && \
       curl -fsSL --retry 3 "$RELEASE_URL/SHA256SUMS" -o "$TMP_DIR/SHA256SUMS" 2>/dev/null; then
      grep " $ASSET\$" "$TMP_DIR/SHA256SUMS" > "$TMP_DIR/$ASSET.sha256"
      (cd "$TMP_DIR" && sha256sum -c "$ASSET.sha256")
    fi
    install -m 755 "$TMP_DIR/$ASSET" /usr/local/bin/entrypool.new
    INSTALLED=1
  else
    echo "Release download failed, trying go build..." >&2
  fi
fi

if [ "$INSTALLED" -eq 0 ] && command -v go >/dev/null 2>&1 && [ -f "$SOURCE_DIR/go.mod" ]; then
  echo "Building from source..."
  (cd "$SOURCE_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
    go build -trimpath -ldflags="-s -w -X main.Version=${ENTRYPOOL_VERSION} -X main.Commit=local" \
    -o "$TMP_DIR/entrypool" .)
  install -m 755 "$TMP_DIR/entrypool" /usr/local/bin/entrypool.new
  INSTALLED=1
fi

if [ "$INSTALLED" -eq 0 ]; then
  echo "Failed to install entrypool binary" >&2
  exit 1
fi

mv -f /usr/local/bin/entrypool.new /usr/local/bin/entrypool

mkdir -p /etc/entrypool /var/lib/entrypool
if [ ! -f /etc/entrypool/config.env ]; then
  if [ -f "$SOURCE_DIR/config.example.env" ]; then
    install -m 600 "$SOURCE_DIR/config.example.env" /etc/entrypool/config.env
  else
    entrypool init-config
  fi
  echo "Created /etc/entrypool/config.env — fill ENTRY_IPS and tokens"
fi

install -m 644 "$SOURCE_DIR/systemd/entrypool.service" /etc/systemd/system/entrypool.service
install -m 644 "$SOURCE_DIR/systemd/entrypool.timer" /etc/systemd/system/entrypool.timer
systemctl daemon-reload

echo
echo "Installed: $(entrypool version)"
echo "Config:    /etc/entrypool/config.env"
echo
echo "Next:"
echo "  nano /etc/entrypool/config.env"
echo "  entrypool doctor"
echo "  systemctl enable --now entrypool.timer"
