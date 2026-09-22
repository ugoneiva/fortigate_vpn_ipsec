package daemon

import (
	"encoding/json"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"fortivpn-client/internal/ipc"
	"fortivpn-client/internal/profile"
)

// Helper is exported on the system bus as ipc.Interface at ipc.ObjectPath.
// Status is intentionally unauthenticated at the polkit layer — it's the
// method the GUI polls every ~2s, and that path must be structurally
// incapable of producing a password prompt. Every other method checks
// polkit via the sender's bus name before touching anything.
type Helper struct {
	conn *dbus.Conn
}

func (h *Helper) Status(connName string) (string, *dbus.Error) {
	st, err := status(connName)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	b, _ := json.Marshal(st)
	return string(b), nil
}

func (h *Helper) ApplyProfile(profileJSON, credsJSON string, sender dbus.Sender) (string, *dbus.Error) {
	if err := h.authorize(sender); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	var p profile.Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("decoding profile: %w", err))
	}
	var creds profile.Credentials
	if err := json.Unmarshal([]byte(credsJSON), &creds); err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("decoding credentials: %w", err))
	}
	if err := applyProfile(p, creds); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return fmt.Sprintf("profile %q applied", p.Name), nil
}

func (h *Helper) Connect(connName string, sender dbus.Sender) (string, *dbus.Error) {
	if err := h.authorize(sender); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	if err := connect(connName); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return "connect initiated", nil
}

func (h *Helper) Disconnect(connName string, sender dbus.Sender) (string, *dbus.Error) {
	if err := h.authorize(sender); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	if err := disconnect(connName); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return "disconnected", nil
}

func (h *Helper) RemoveProfile(connName string, sender dbus.Sender) (string, *dbus.Error) {
	if err := h.authorize(sender); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	if err := removeProfile(connName); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return "profile removed", nil
}

func (h *Helper) authorize(sender dbus.Sender) error {
	return checkAuthorization(h.conn, string(sender), ipc.ActionManage)
}

// Run connects to the system bus, exports the Helper, and serves requests
// until the process is killed (systemd stops it after its own idle policy,
// or on failure).
func Run() error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("connecting to system bus: %w", err)
	}
	defer conn.Close()

	helper := &Helper{conn: conn}
	if err := conn.Export(helper, ipc.ObjectPath, ipc.Interface); err != nil {
		return fmt.Errorf("exporting D-Bus methods: %w", err)
	}
	if err := conn.Export(introspect.NewIntrospectable(introspectNode()), ipc.ObjectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("exporting introspection data: %w", err)
	}

	reply, err := conn.RequestName(ipc.BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("requesting bus name %s: %w", ipc.BusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("bus name %s already owned by another process", ipc.BusName)
	}

	notifySystemdReady()

	select {} // serve forever; godbus dispatches incoming calls on its own goroutines
}

func introspectNode() *introspect.Node {
	arg := func(name, typ, dir string) introspect.Arg {
		return introspect.Arg{Name: name, Type: typ, Direction: dir}
	}
	return &introspect.Node{
		Name: string(ipc.ObjectPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name: ipc.Interface,
				Methods: []introspect.Method{
					{Name: "Status", Args: []introspect.Arg{
						arg("connName", "s", "in"), arg("statusJSON", "s", "out"),
					}},
					{Name: "ApplyProfile", Args: []introspect.Arg{
						arg("profileJSON", "s", "in"), arg("credsJSON", "s", "in"), arg("message", "s", "out"),
					}},
					{Name: "Connect", Args: []introspect.Arg{
						arg("connName", "s", "in"), arg("message", "s", "out"),
					}},
					{Name: "Disconnect", Args: []introspect.Arg{
						arg("connName", "s", "in"), arg("message", "s", "out"),
					}},
					{Name: "RemoveProfile", Args: []introspect.Arg{
						arg("connName", "s", "in"), arg("message", "s", "out"),
					}},
				},
			},
		},
	}
}
