// Package guiapp is the unprivileged half of fortivpn-client: the Fyne GUI,
// tray icon, and a D-Bus client that talks to fortivpn-daemon over the
// system bus. Nothing in this package invokes pkexec — every privileged
// action goes through the daemon, which handles its own polkit prompting.
package guiapp

import (
	"encoding/json"
	"fmt"

	"github.com/godbus/dbus/v5"

	"fortivpn-client/internal/ipc"
	"fortivpn-client/internal/profile"
)

// DaemonClient is a thin wrapper over the four D-Bus methods
// fortivpn-daemon exports. Every call here may block waiting on a polkit
// prompt (for the mutating methods) or on swanctl (for all of them) — always
// call from a goroutine, never the Fyne UI goroutine.
type DaemonClient struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

func NewDaemonClient() (*DaemonClient, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("connecting to system bus: %w", err)
	}
	obj := conn.Object(ipc.BusName, ipc.ObjectPath)
	return &DaemonClient{conn: conn, obj: obj}, nil
}

func (c *DaemonClient) Close() error {
	return c.conn.Close()
}

// Status calls the daemon's unauthenticated status query. Safe to call as
// often as needed (this is what app.go's ~2s ticker does) — it has no
// polkit check and therefore no path to a password prompt.
func (c *DaemonClient) Status(connName string) (profile.ConnectionStatus, error) {
	var raw string
	if err := c.obj.Call(ipc.Interface+".Status", 0, connName).Store(&raw); err != nil {
		return profile.ConnectionStatus{}, err
	}
	var st profile.ConnectionStatus
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return profile.ConnectionStatus{}, fmt.Errorf("decoding status: %w", err)
	}
	return st, nil
}

func (c *DaemonClient) ApplyProfile(p profile.Profile, creds profile.Credentials) (string, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encoding profile: %w", err)
	}
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("encoding credentials: %w", err)
	}
	var msg string
	err = c.obj.Call(ipc.Interface+".ApplyProfile", 0, string(profileJSON), string(credsJSON)).Store(&msg)
	return msg, err
}

func (c *DaemonClient) Connect(connName string) (string, error) {
	var msg string
	err := c.obj.Call(ipc.Interface+".Connect", 0, connName).Store(&msg)
	return msg, err
}

func (c *DaemonClient) Disconnect(connName string) (string, error) {
	var msg string
	err := c.obj.Call(ipc.Interface+".Disconnect", 0, connName).Store(&msg)
	return msg, err
}

func (c *DaemonClient) RemoveProfile(connName string) (string, error) {
	var msg string
	err := c.obj.Call(ipc.Interface+".RemoveProfile", 0, connName).Store(&msg)
	return msg, err
}
