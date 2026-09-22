package daemon

import (
	"net"
	"os"
)

// notifySystemdReady tells systemd (Type=notify unit) that the D-Bus name is
// now owned and requests can be served. A no-op outside systemd (e.g. a
// manual `sudo ./fortivpn-daemon` during development, where NOTIFY_SOCKET
// isn't set).
func notifySystemdReady() {
	addr := os.Getenv("NOTIFY_SOCKET")
	if addr == "" {
		return
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("READY=1"))
}
