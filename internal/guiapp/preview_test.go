package guiapp

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"fortivpn-client/internal/profile"
)

// TestPreviewWindow renders the main window with no display attached, so the
// look can be reviewed during development. It asserts nothing; it writes PNGs.
// Set PREVIEW_DIR to enable it.
func TestPreviewWindow(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("PREVIEW_DIR not set")
	}
	test.NewApp()
	fyne.CurrentApp().Settings().SetTheme(vpnTheme{})

	a := &App{
		// Documentation addresses (RFC 5737) and generic names: this repository
		// is public and must never carry a real customer gateway.
		store: profile.Store{Profiles: []profile.Profile{
			{ID: "1", Name: "Head Office", Gateway: "203.0.113.10", IkeVersion: profile.IkeV1, Ikev1Mode: profile.Ikev1Aggressive},
			{ID: "2", Name: "Datacenter", Gateway: "203.0.113.24", IkeVersion: profile.IkeV1, Ikev1Mode: profile.Ikev1Aggressive},
			{ID: "3", Name: "Branch Office", Gateway: "198.51.100.7", IkeVersion: profile.IkeV1, Ikev1Mode: profile.Ikev1Main},
			{ID: "4", Name: "Partner Site", Gateway: "192.0.2.45", IkeVersion: profile.IkeV2},
		}},
		selected: -1,
		states:   map[string]profile.ConnState{},
	}
	w := test.NewWindow(nil)
	a.window = w
	a.buildUI()

	var scenes []struct {
		name  string
		setup func()
	}
	scenes = []struct {
		name  string
		setup func()
	}{
		{"1-no-selection", func() {}},
		{"2-connected", func() {
			a.selected = 0
			a.setActionButtonsEnabled(true)
			p := a.store.Profiles[0]
			a.states[p.ConnName()] = profile.StateConnected
			a.states[a.store.Profiles[1].ConnName()] = profile.StateNoTunnel
			a.card.update(p.Name, profile.ConnectionStatus{State: profile.StateConnected, VirtualIP: "10.0.0.101"}, true)
			a.reflectState(profile.StateConnected)
			a.list.Refresh()
		}},
		{"3-no-tunnel", func() {
			a.selected = 1
			p := a.store.Profiles[1]
			a.card.update(p.Name, profile.ConnectionStatus{State: profile.StateNoTunnel, VirtualIP: "10.0.0.42"}, true)
			a.reflectState(profile.StateNoTunnel)
			a.list.Refresh()
		}},
	}
	// The profile form inherits the theme; worth checking it stayed legible.
	scenes = append(scenes, struct {
		name  string
		setup func()
	}{"4-form", func() {
		p := a.store.Profiles[0]
		a.showProfileForm(&p)
	}})

	for _, c := range scenes {
		c.setup()
		w.Resize(fyne.NewSize(640, 640))
		img := w.Canvas().Capture()
		f, err := os.Create(dir + "/" + c.name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
		t.Log("wrote", c.name)
	}
}
