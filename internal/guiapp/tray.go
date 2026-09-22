package guiapp

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// setupTray uses Fyne's native desktop.App tray support instead of a
// separate systray dependency, so there's a single event loop and no extra
// cgo tray package to coordinate.
func (a *App) setupTray() {
	desk, ok := a.fyneApp.(desktop.App)
	if !ok {
		return // platform without tray support; window-only fallback
	}
	menu := fyne.NewMenu("FortiVPN Client",
		fyne.NewMenuItem("Show", func() { a.window.Show() }),
		fyne.NewMenuItem("Connect", a.connectSelected),
		fyne.NewMenuItem("Disconnect", a.disconnectSelected),
		fyne.NewMenuItem("Quit", a.fyneApp.Quit),
	)
	desk.SetSystemTrayMenu(menu)
	desk.SetSystemTrayIcon(appIcon)
}
