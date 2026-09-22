package daemon

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	polkitDest = "org.freedesktop.PolicyKit1"
	polkitPath = dbus.ObjectPath("/org/freedesktop/PolicyKit1/Authority")

	// checkAuthorizationAllowInteraction: without this bit, CheckAuthorization
	// fails closed instead of prompting — every mutating call needs it since
	// that prompt (at most one, then polkit's own auth_admin_keep cache
	// covers subsequent calls) is the intended, sole password prompt in this
	// whole application.
	checkAuthorizationAllowInteraction = 1
)

type polkitSubject struct {
	Kind    string
	Details map[string]dbus.Variant
}

type polkitResult struct {
	IsAuthorized bool
	IsChallenge  bool
	Details      map[string]string
}

// checkAuthorization asks polkit whether the D-Bus caller identified by
// sender (its unique bus name, e.g. ":1.234", injected by godbus via a
// dbus.Sender parameter) is authorized for actionID, prompting them for a
// password if needed. This is the entire privilege-escalation surface of
// fortivpn-daemon — there is no pkexec anywhere in this codebase anymore.
func checkAuthorization(sysConn *dbus.Conn, sender, actionID string) error {
	subject := polkitSubject{
		Kind:    "system-bus-name",
		Details: map[string]dbus.Variant{"name": dbus.MakeVariant(sender)},
	}

	var result polkitResult
	obj := sysConn.Object(polkitDest, polkitPath)
	err := obj.Call("org.freedesktop.PolicyKit1.Authority.CheckAuthorization", 0,
		subject, actionID, map[string]string{}, uint32(checkAuthorizationAllowInteraction), "").Store(&result)
	if err != nil {
		return fmt.Errorf("polkit CheckAuthorization: %w", err)
	}
	if !result.IsAuthorized {
		return fmt.Errorf("not authorized")
	}
	return nil
}
