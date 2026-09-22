package guiapp

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fortivpn-client/internal/profile"
)

// stateAppearance maps a connection state to the colour and label used for it.
// This is the only source of both: the status card, the list dots and the
// action button all go through here, so one situation can never show up in two
// different colours.
func stateAppearance(s profile.ConnState) (color.NRGBA, string) {
	switch s {
	case profile.StateConnected:
		return connectedColor, "Connected"
	case profile.StateNoTunnel:
		return attentionColor, "Phase 1 up, no tunnel"
	case profile.StateConnecting:
		return connectingColor, "Connecting…"
	default:
		return idleColor, "Disconnected"
	}
}

// setText replaces the content of a canvas.Text, which has no SetText.
func setText(t *canvas.Text, s string) {
	t.Text = s
	t.Refresh()
}

// surfaceRect is the rounded rectangle used as a card background.
func surfaceRect(fill color.Color) *canvas.Rectangle {
	r := canvas.NewRectangle(fill)
	r.CornerRadius = 12
	return r
}

// statusDot is the coloured circle that shows a profile's state.
type statusDot struct {
	widget.BaseWidget
	halo *canvas.Circle
	core *canvas.Circle
}

func newStatusDot() *statusDot {
	d := &statusDot{
		halo: canvas.NewCircle(color.NRGBA{}),
		core: canvas.NewCircle(idleColor),
	}
	d.ExtendBaseWidget(d)
	d.setColor(idleColor)
	return d
}

// setColor paints the dot. The halo is the same colour at low alpha, which
// reads as a glow without needing a shadow.
func (d *statusDot) setColor(c color.NRGBA) {
	d.core.FillColor = c
	d.halo.FillColor = color.NRGBA{R: c.R, G: c.G, B: c.B, A: 0x40}
	d.core.Refresh()
	d.halo.Refresh()
}

func (d *statusDot) setState(s profile.ConnState) {
	c, _ := stateAppearance(s)
	d.setColor(c)
}

func (d *statusDot) CreateRenderer() fyne.WidgetRenderer {
	return &statusDotRenderer{dot: d, objs: []fyne.CanvasObject{d.halo, d.core}}
}

type statusDotRenderer struct {
	dot  *statusDot
	objs []fyne.CanvasObject
}

func (r *statusDotRenderer) Layout(s fyne.Size) {
	side := fyne.Min(s.Width, s.Height)
	r.dot.halo.Resize(fyne.NewSize(side, side))
	r.dot.halo.Move(fyne.NewPos((s.Width-side)/2, (s.Height-side)/2))
	core := side * 0.5
	r.dot.core.Resize(fyne.NewSize(core, core))
	r.dot.core.Move(fyne.NewPos((s.Width-core)/2, (s.Height-core)/2))
}
func (r *statusDotRenderer) MinSize() fyne.Size           { return fyne.NewSize(16, 16) }
func (r *statusDotRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *statusDotRenderer) Refresh()                     { canvas.Refresh(r.dot) }
func (r *statusDotRenderer) Destroy()                     {}

// statusCard is the block at the top of the window: the state in large type,
// the profile name and the connection details.
type statusCard struct {
	root        *fyne.Container
	stripe      *canvas.Rectangle
	dot         *statusDot
	state       *canvas.Text
	profileName *canvas.Text
	detail      *canvas.Text
}

func newStatusCard() *statusCard {
	c := &statusCard{
		stripe:      canvas.NewRectangle(idleColor),
		dot:         newStatusDot(),
		state:       canvas.NewText("", theme.Color(theme.ColorNameForeground)),
		profileName: canvas.NewText("", theme.Color(theme.ColorNameForeground)),
		detail:      canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder)),
	}
	c.stripe.CornerRadius = 3
	// A canvas.Rectangle has a zero MinSize, so without this the Border layout
	// would give the stripe no width at all.
	c.stripe.SetMinSize(fyne.NewSize(4, 0))
	c.state.TextStyle = fyne.TextStyle{Bold: true}
	c.state.TextSize = 20
	c.profileName.TextStyle = fyne.TextStyle{Bold: true}
	c.profileName.TextSize = 15
	c.detail.TextSize = 13

	text := container.NewVBox(
		container.NewHBox(c.dot, c.state),
		c.profileName,
		c.detail,
	)
	// The coloured stripe repeats the state in a form that reads at a glance,
	// without having to take in the text.
	body := container.NewBorder(nil, nil, c.stripe, nil, container.NewPadded(text))
	c.root = container.NewStack(surfaceRect(theme.Color(theme.ColorNameOverlayBackground)),
		container.NewPadded(body))

	// Explicit initial state: without this the card starts blank and only gets
	// its text on the first polling tick, ~2s after the window opens.
	c.update("", profile.ConnectionStatus{}, false)
	return c
}

func (c *statusCard) update(name string, st profile.ConnectionStatus, hasProfile bool) {
	if !hasProfile {
		c.dot.setState(profile.StateDisconnected)
		c.stripe.FillColor = idleColor
		c.state.Text = "No profile selected"
		c.state.Color = theme.Color(theme.ColorNamePlaceHolder)
		setText(c.profileName, "")
		setText(c.detail, "Select a profile from the list below")
		c.stripe.Refresh()
		c.state.Refresh()
		return
	}
	col, label := stateAppearance(st.State)
	c.dot.setState(st.State)
	c.stripe.FillColor = col
	c.state.Text = label
	c.state.Color = col
	setText(c.profileName, name)
	if st.VirtualIP != "" {
		setText(c.detail, "VPN address: "+st.VirtualIP)
	} else {
		setText(c.detail, "No address assigned")
	}
	c.stripe.Refresh()
	c.state.Refresh()
}

// showError reports a status lookup failure without disturbing the rest of the
// card.
func (c *statusCard) showError(msg string) {
	c.dot.setColor(failureColor)
	c.stripe.FillColor = failureColor
	c.state.Text = "Error"
	c.state.Color = failureColor
	setText(c.detail, msg)
	c.stripe.Refresh()
	c.state.Refresh()
}
