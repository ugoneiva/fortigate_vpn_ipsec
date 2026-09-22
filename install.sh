#!/usr/bin/env bash
set -euo pipefail

# Installs fortivpn-client on any systemd + D-Bus + polkit Linux distro
# (Arch users may prefer packaging/PKGBUILD via `makepkg -si` instead, for
# a package-manager-tracked install). Builds the Go binaries as the
# invoking user, then uses sudo only for the actual system-file copy steps.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [ "$(id -u)" -eq 0 ]; then
  echo "Run this as your normal user, not root — it uses sudo internally" >&2
  echo "for the install steps, and needs your own Go module cache to build." >&2
  exit 1
fi

missing=0
need() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing required tool: $1" >&2; missing=1; }
}
need go
need gcc
need pkg-config
need systemctl
need swanctl

if [ "$missing" -eq 1 ]; then
  cat >&2 <<'EOF'

Install the missing tools via your distro's package manager, e.g.:

  Arch/derivatives:  sudo pacman -S go gcc pkgconf strongswan polkit dbus gtk3
  Debian/Ubuntu:     sudo apt install golang-go gcc pkg-config strongswan-swanctl policykit-1 libgtk-3-dev
  Fedora:            sudo dnf install golang gcc pkgconf-pkg-config strongswan polkit gtk3-devel

Then re-run this script.
EOF
  exit 1
fi

BUILD_DIR="$SCRIPT_DIR/.installbuild"
mkdir -p "$BUILD_DIR"
trap 'rm -rf "$BUILD_DIR"' EXIT

echo "==> Building fortivpn-daemon and fortivpn-gui"
go build -o "$BUILD_DIR/fortivpn-daemon" ./cmd/fortivpn-daemon
go build -o "$BUILD_DIR/fortivpn-gui" ./cmd/fortivpn-gui

echo "==> Installing to /usr and /etc (will prompt for your sudo password)"
sudo install -Dm755 "$BUILD_DIR/fortivpn-daemon" /usr/lib/fortivpn-client/fortivpn-daemon
sudo install -Dm755 "$BUILD_DIR/fortivpn-gui" /usr/bin/fortivpn-gui
sudo install -Dm644 packaging/fortivpn-daemon.service /usr/lib/systemd/system/fortivpn-daemon.service
sudo install -Dm644 packaging/dev.fortivpn.Helper1.service /usr/share/dbus-1/system-services/dev.fortivpn.Helper1.service
sudo install -Dm644 packaging/dev.fortivpn.Helper1.conf /usr/share/dbus-1/system.d/dev.fortivpn.Helper1.conf
sudo install -Dm644 packaging/dev.fortivpn.helper.policy /usr/share/polkit-1/actions/dev.fortivpn.helper.policy
sudo install -Dm644 packaging/fortivpn-client.desktop /usr/share/applications/fortivpn-client.desktop
sudo install -Dm644 internal/guiapp/assets/icon.svg /usr/share/icons/hicolor/scalable/apps/fortivpn-client.svg

echo "==> Reloading systemd and D-Bus"
sudo systemctl daemon-reload
sudo systemctl try-restart fortivpn-daemon.service 2>/dev/null || true
sudo systemctl reload dbus-broker.service 2>/dev/null || sudo systemctl reload dbus.service 2>/dev/null || true

command -v gtk-update-icon-cache >/dev/null 2>&1 && sudo gtk-update-icon-cache -f /usr/share/icons/hicolor >/dev/null 2>&1 || true
command -v update-desktop-database >/dev/null 2>&1 && sudo update-desktop-database /usr/share/applications >/dev/null 2>&1 || true

# FortiGate dialup is IKEv1 (usually Aggressive Mode + XAuth). strongSwan 6.x
# builds charon WITHOUT IKEv1 by default, and PSK+XAuth also needs the
# xauth-generic plugin. Check both and say what to do.
LIBCHARON=$(ls /usr/lib/ipsec/libcharon.so.0 /usr/lib/*/ipsec/libcharon.so.0 \
  /usr/lib64/ipsec/libcharon.so.0 /usr/libexec/strongswan/libcharon.so.0 2>/dev/null | head -1)
if [ -n "$LIBCHARON" ] && ! grep -q "XAuth authentication of" "$LIBCHARON"; then
  cat >&2 <<'EOF2'

WARNING: this strongSwan was built without IKEv1 support (strongSwan 6.x
disables it by default). IKEv1 profiles — the usual FortiGate dialup setup —
will not connect.

  Arch/derivatives:  (cd packaging/strongswan-ikev1 && makepkg -si)
                     back up /etc/swanctl and /etc/strongswan.conf first,
                     then: sudo systemctl restart strongswan
  Other distros:     install or build strongSwan with --enable-ikev1

EOF2
elif ! ls /usr/lib/ipsec/plugins/libstrongswan-xauth-generic.so \
        /usr/lib/*/ipsec/plugins/libstrongswan-xauth-generic.so \
        /usr/lib64/ipsec/plugins/libstrongswan-xauth-generic.so >/dev/null 2>&1; then
  cat >&2 <<'EOF2'

WARNING: strongSwan's xauth-generic plugin was not found. "PSK + XAuth"
profiles (FortiGate IKEv1 dialup) will fail to authenticate without it.

  Arch/derivatives:  packaging/strongswan-ikev1 includes it
  Debian/Ubuntu:     sudo apt install libcharon-extra-plugins

EOF2
fi

echo "==> Done. Launch 'fortivpn-gui' or find FortiVPN Client in your app menu."
