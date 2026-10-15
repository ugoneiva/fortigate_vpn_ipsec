package guiapp

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fortivpn-client/internal/profile"
	"fortivpn-client/internal/saml"
)

type App struct {
	fyneApp fyne.App
	window  fyne.Window
	client  *DaemonClient

	store    profile.Store
	selected int // index into store.Profiles, -1 if none

	list      *widget.List
	card      *statusCard
	actionBtn *widget.Button
	editBtn   *widget.Button
	removeBtn *widget.Button

	// Per-profile state, fed by the polling loop, for the list dots.
	states map[string]profile.ConnState
	// State of the selected profile, which decides what the primary button does.
	currentState profile.ConnState
}

func Run() error {
	client, err := NewDaemonClient()
	if err != nil {
		return fmt.Errorf("connecting to fortivpn-daemon: %w", err)
	}

	store, err := profile.LoadStore()
	if err != nil {
		client.Close()
		return fmt.Errorf("loading profiles: %w", err)
	}

	a := &App{
		fyneApp:  app.NewWithID("dev.fortivpn.client"),
		client:   client,
		store:    store,
		selected: -1,
		states:   map[string]profile.ConnState{},
	}
	a.fyneApp.SetIcon(appIcon)
	a.fyneApp.Settings().SetTheme(vpnTheme{})

	a.window = a.fyneApp.NewWindow("FortiVPN Client")
	a.window.SetIcon(appIcon)
	a.buildUI()
	a.setupTray()
	a.startStatusPolling()

	// Hide to tray instead of quitting on window close — the tray menu's
	// "Quit" is the only way out, matching a normal desktop VPN client.
	a.window.SetCloseIntercept(func() { a.window.Hide() })

	a.window.Resize(fyne.NewSize(640, 600))
	a.window.ShowAndRun()

	client.Close()
	return nil
}

func (a *App) buildUI() {
	a.list = a.newProfileList()
	a.card = newStatusCard()

	// A single primary button: connect or disconnect depending on the state.
	// Two buttons always on screen, one of them useless, was what dated the
	// window the most.
	a.actionBtn = widget.NewButtonWithIcon("Connect", theme.MediaPlayIcon(), a.primaryAction)
	a.actionBtn.Importance = widget.HighImportance

	addBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() { a.showProfileForm(nil) })
	addBtn.Importance = widget.LowImportance
	a.editBtn = widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), a.editSelected)
	a.editBtn.Importance = widget.LowImportance
	a.removeBtn = widget.NewButtonWithIcon("", theme.DeleteIcon(), a.removeSelected)
	a.removeBtn.Importance = widget.LowImportance
	a.setActionButtonsEnabled(false)

	title := canvas.NewText("FortiVPN", theme.Color(theme.ColorNameForeground))
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 20
	sub := canvas.NewText("IPsec client for FortiGate", theme.Color(theme.ColorNamePlaceHolder))
	sub.TextSize = 12
	brand := container.NewHBox(
		container.NewGridWrap(fyne.NewSize(32, 32), canvas.NewImageFromResource(appIcon)),
		container.NewVBox(layout.NewSpacer(), title, sub, layout.NewSpacer()),
	)
	header := container.NewBorder(nil, nil, brand, container.NewHBox(addBtn, a.editBtn, a.removeBtn))

	listLabel := canvas.NewText("PROFILES", theme.Color(theme.ColorNamePlaceHolder))
	listLabel.TextStyle = fyne.TextStyle{Bold: true}
	listLabel.TextSize = 11

	top := container.NewVBox(
		container.NewPadded(header),
		a.card.root,
		container.NewPadded(listLabel),
	)
	footer := container.NewPadded(a.actionBtn)

	a.window.SetContent(container.NewPadded(
		container.NewBorder(top, footer, nil, nil, a.list),
	))
}

// primaryAction disconnects when there is a tunnel (or a dangling phase 1)
// and connects in every other case.
func (a *App) primaryAction() {
	switch a.currentState {
	case profile.StateConnected, profile.StateNoTunnel, profile.StateConnecting:
		a.disconnectSelected()
	default:
		a.connectSelected()
	}
}

// reflectState keeps the primary button consistent with the profile state.
func (a *App) reflectState(s profile.ConnState) {
	a.currentState = s
	switch s {
	case profile.StateConnected, profile.StateNoTunnel:
		a.actionBtn.SetText("Disconnect")
		a.actionBtn.SetIcon(theme.MediaStopIcon())
		a.actionBtn.Importance = widget.DangerImportance
	case profile.StateConnecting:
		a.actionBtn.SetText("Cancel")
		a.actionBtn.SetIcon(theme.MediaStopIcon())
		a.actionBtn.Importance = widget.MediumImportance
	default:
		a.actionBtn.SetText("Connect")
		a.actionBtn.SetIcon(theme.MediaPlayIcon())
		a.actionBtn.Importance = widget.HighImportance
	}
	a.actionBtn.Refresh()
}

func (a *App) setActionButtonsEnabled(enabled bool) {
	set := func(b *widget.Button) {
		if enabled {
			b.Enable()
		} else {
			b.Disable()
		}
	}
	set(a.editBtn)
	set(a.removeBtn)
	set(a.actionBtn)
}

func (a *App) selectedProfile() (profile.Profile, bool) {
	if a.selected < 0 || a.selected >= len(a.store.Profiles) {
		return profile.Profile{}, false
	}
	return a.store.Profiles[a.selected], true
}

func (a *App) editSelected() {
	if p, ok := a.selectedProfile(); ok {
		a.showProfileForm(&p)
	}
}

func (a *App) connectSelected() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	a.reflectState(profile.StateConnecting)
	a.card.update(p.Name, profile.ConnectionStatus{State: profile.StateConnecting}, true)
	if p.AuthMode.IsSAML() {
		go a.connectSAML(p)
		return
	}
	go func() {
		if _, err := a.client.Connect(p.ConnName()); err != nil {
			fyne.Do(func() { dialog.ShowError(err, a.window) })
		}
	}()
}

// connectSAML runs the browser login and connects with the one-time token.
// The token goes straight into ApplyProfile (rewriting the 0600 secrets file)
// and is never written to the keyring: every Connect needs a fresh login.
func (a *App) connectSAML(p profile.Profile) {
	fail := func(err error) {
		fyne.Do(func() {
			dialog.ShowError(err, a.window)
			a.reflectState(profile.StateDisconnected)
		})
	}
	creds, err := LoadCredentials(p.ConnName())
	if err != nil || creds.PSK == "" {
		fail(fmt.Errorf("no pre-shared key saved for %s — edit the profile and set the PSK", p.Name))
		return
	}
	host, port := p.SAMLEndpoint()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := saml.Begin(ctx, nil, host, port)
	if err != nil {
		fail(err)
		return
	}
	var waiting dialog.Dialog
	fyne.Do(func() {
		msg := widget.NewLabel(fmt.Sprintf("Complete the login in your browser (%s).\nThis window closes by itself when the gateway answers.", host))
		waiting = dialog.NewCustom("SAML login", "Cancel", msg, a.window)
		waiting.SetOnClosed(cancel)
		waiting.Show()
		if u, err := url.Parse(sess.LoginURL); err == nil {
			_ = a.fyneApp.OpenURL(u)
		}
	})
	res, err := sess.Wait(ctx)
	fyne.Do(func() {
		if waiting != nil {
			// Do NOT SetOnClosed(nil): fyne 2.8 wraps the callback and calls it
			// on Hide, so a nil callback is a nil-pointer panic that kills the
			// whole GUI. The callback is cancel(), harmless once Wait returned.
			waiting.Hide()
		}
	})
	if err != nil {
		fail(err)
		return
	}
	creds.Username, creds.Password = res.UID, res.Token
	if _, err := a.client.ApplyProfile(p, creds); err != nil {
		fail(fmt.Errorf("applying profile with the SAML token: %w", err))
		return
	}
	if _, err := a.client.Connect(p.ConnName()); err != nil {
		fail(err)
	}
}

func (a *App) disconnectSelected() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	go func() {
		if _, err := a.client.Disconnect(p.ConnName()); err != nil {
			fyne.Do(func() { dialog.ShowError(err, a.window) })
		}
	}()
}

func (a *App) removeSelected() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	idx := a.selected
	dialog.ShowConfirm("Remove profile", fmt.Sprintf("Remove %q?", p.Name), func(confirmed bool) {
		if !confirmed {
			return
		}
		go func() {
			if _, err := a.client.RemoveProfile(p.ConnName()); err != nil {
				fyne.Do(func() { dialog.ShowError(err, a.window) })
				return
			}
			_ = DeleteCredentials(p.ConnName())
			fyne.Do(func() {
				a.store.Profiles = append(a.store.Profiles[:idx], a.store.Profiles[idx+1:]...)
				if err := profile.SaveStore(a.store); err != nil {
					dialog.ShowError(err, a.window)
				}
				a.selected = -1
				a.list.UnselectAll()
				a.list.Refresh()
				a.setActionButtonsEnabled(false)
			})
		}()
	}, a.window)
}

// saveProfile applies the profile (writing swanctl conf.d — a privileged,
// polkit-gated call) and persists credentials/profile list only after that
// succeeds, so profiles.toml never references a profile the daemon doesn't
// actually know about.
func (a *App) saveProfile(p profile.Profile, creds profile.Credentials, isNew bool) {
	go func() {
		if err := SaveCredentials(p.ConnName(), creds); err != nil {
			fyne.Do(func() { dialog.ShowError(fmt.Errorf("saving credentials: %w", err), a.window) })
			return
		}
		if _, err := a.client.ApplyProfile(p, creds); err != nil {
			fyne.Do(func() { dialog.ShowError(fmt.Errorf("applying profile: %w", err), a.window) })
			return
		}
		fyne.Do(func() {
			if isNew {
				a.store.Profiles = append(a.store.Profiles, p)
			} else {
				for i := range a.store.Profiles {
					if a.store.Profiles[i].ID == p.ID {
						a.store.Profiles[i] = p
						break
					}
				}
			}
			if err := profile.SaveStore(a.store); err != nil {
				dialog.ShowError(fmt.Errorf("saving profile list: %w", err), a.window)
			}
			a.list.Refresh()
		})
	}()
}
