// Package ipc holds the D-Bus identifiers shared by fortivpn-daemon (which
// exports them) and fortivpn-gui (which calls them) — kept in one place so
// the two binaries can't drift out of sync.
package ipc

import "github.com/godbus/dbus/v5"

const (
	BusName    = "dev.fortivpn.Helper1"
	ObjectPath = dbus.ObjectPath("/dev/fortivpn/Helper1")
	Interface  = "dev.fortivpn.Helper1"

	// ActionManage gates every state-changing method (ApplyProfile, Connect,
	// Disconnect, RemoveProfile). Status is deliberately NOT gated by any
	// polkit action — see internal/daemon/service.go.
	ActionManage = "dev.fortivpn.helper.manage"
)
