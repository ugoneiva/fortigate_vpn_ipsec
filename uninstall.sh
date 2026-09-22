#!/usr/bin/env bash
set -euo pipefail

# Reverses install.sh. Leaves ~/.config/fortivpn-client/profiles.toml, any
# saved credentials in the Secret Service keyring, and any
# /etc/swanctl/conf.d/fortivpn-*.conf files an applied profile wrote —
# none of those are "installed files," they're user data, so this doesn't
# touch them without being asked to.

if [ "$(id -u)" -eq 0 ]; then
  echo "Run this as your normal user — it uses sudo internally." >&2
  exit 1
fi

echo "==> Stopping fortivpn-daemon"
sudo systemctl stop fortivpn-daemon.service 2>/dev/null || true

echo "==> Removing installed files (will prompt for your sudo password)"
sudo rm -f /usr/lib/fortivpn-client/fortivpn-daemon
sudo rmdir /usr/lib/fortivpn-client 2>/dev/null || true
sudo rm -f /usr/bin/fortivpn-gui
sudo rm -f /usr/lib/systemd/system/fortivpn-daemon.service
sudo rm -f /usr/share/dbus-1/system-services/dev.fortivpn.Helper1.service
sudo rm -f /usr/share/dbus-1/system.d/dev.fortivpn.Helper1.conf
sudo rm -f /usr/share/polkit-1/actions/dev.fortivpn.helper.policy
sudo rm -f /usr/share/applications/fortivpn-client.desktop
sudo rm -f /usr/share/icons/hicolor/scalable/apps/fortivpn-client.svg

echo "==> Reloading systemd and D-Bus"
sudo systemctl daemon-reload
sudo systemctl reload dbus-broker.service 2>/dev/null || sudo systemctl reload dbus.service 2>/dev/null || true

cat <<'EOF'
==> Done.

Not removed (user data, not installed files):
  - ~/.config/fortivpn-client/profiles.toml
  - credentials saved in your Secret Service keyring
  - /etc/swanctl/conf.d/fortivpn-*.conf for any profile still applied
    (open the app and remove profiles first if you want those cleaned up)
EOF
