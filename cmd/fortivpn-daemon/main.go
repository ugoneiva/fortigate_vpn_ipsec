// fortivpn-daemon is the privileged half of fortivpn-client: a D-Bus system
// service (see internal/daemon) started by D-Bus activation, never invoked
// directly by the GUI or by pkexec.
package main

import (
	"fmt"
	"os"

	"fortivpn-client/internal/daemon"
)

func main() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "fortivpn-daemon must run as root (started via systemd/D-Bus activation)")
		os.Exit(1)
	}
	if err := daemon.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fortivpn-daemon: %v\n", err)
		os.Exit(1)
	}
}
