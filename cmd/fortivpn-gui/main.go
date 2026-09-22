// fortivpn-gui is the unprivileged desktop client: profile management, tray
// icon, and status display. It never invokes pkexec — see
// internal/guiapp/client.go for how it talks to fortivpn-daemon over D-Bus.
package main

import (
	"fmt"
	"os"

	"fortivpn-client/internal/guiapp"
)

func main() {
	if err := guiapp.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fortivpn-gui: %v\n", err)
		os.Exit(1)
	}
}
