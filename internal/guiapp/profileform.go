package guiapp

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"fortivpn-client/internal/profile"
)

var proposalCiphers = []string{"aes128", "aes192", "aes256"}
var proposalIntegrities = []string{"sha1", "sha256", "sha384", "sha512"}

// showProfileForm opens the Add/Edit dialog. existing == nil means Add.
// The IKE/ESP proposal pickers are cipher, integrity and DH-group
// checklists (groups from profile.DHGroups); the saved proposal list is
// their cross product, one string per combination. All three are
// multi-select because FortiClient profiles routinely offer several
// transforms at once (e.g. AES128|SHA1 *and* AES256|SHA256), and a
// single-choice dropdown silently collapsed them to the first one.
func (a *App) showProfileForm(existing *profile.Profile) {
	isNew := existing == nil
	var p profile.Profile
	var creds profile.Credentials
	if existing != nil {
		p = *existing
		if c, err := LoadCredentials(p.ConnName()); err == nil {
			creds = c
		}
	} else {
		p = profile.Profile{
			ID:          profile.NewID(),
			IkeVersion:  profile.IkeV2,
			AuthMode:    profile.AuthEapMschapv2,
			DPDDelay:    "30s",
			SplitTunnel: profile.SplitTunnel{Kind: profile.SplitTunnelFull},
		}
	}

	nameEntry := widget.NewEntry()
	nameEntry.SetText(p.Name)
	gatewayEntry := widget.NewEntry()
	gatewayEntry.SetText(p.Gateway)
	localIDEntry := widget.NewEntry()
	localIDEntry.SetPlaceHolder("optional — FortiClient \"Local ID\"")
	localIDEntry.SetText(p.LocalID)
	remoteIDEntry := widget.NewEntry()
	remoteIDEntry.SetPlaceHolder("optional — FortiClient \"Peer ID\"; blank accepts any")
	remoteIDEntry.SetText(p.RemoteID)

	dhLabels, dhByLabel := dhGroupOptions()

	ikeCipher := horizontalCheckGroup(proposalCiphers)
	ikeIntegrity := horizontalCheckGroup(proposalIntegrities)
	ikeDHGroups := widget.NewCheckGroup(dhLabels, nil)
	espCipher := horizontalCheckGroup(proposalCiphers)
	espIntegrity := horizontalCheckGroup(proposalIntegrities)
	espDHGroups := widget.NewCheckGroup(dhLabels, nil)

	seedProposalPickers(p.Proposals.IKE, ikeCipher, ikeIntegrity, ikeDHGroups)
	seedProposalPickers(p.Proposals.ESP, espCipher, espIntegrity, espDHGroups)
	defaultIfEmpty(ikeCipher, "aes256")
	defaultIfEmpty(ikeIntegrity, "sha256")
	defaultIfEmpty(espCipher, "aes256")
	defaultIfEmpty(espIntegrity, "sha256")

	ikeVersionRadio := widget.NewRadioGroup([]string{"IKEv1", "IKEv2"}, nil)
	ikev1ModeRadio := widget.NewRadioGroup([]string{"Main", "Aggressive"}, nil)
	authModeSelect := widget.NewSelect(nil, nil)

	pskEntry := widget.NewPasswordEntry()
	pskEntry.SetText(creds.PSK)
	usernameEntry := widget.NewEntry()
	usernameEntry.SetText(creds.Username)
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetText(creds.Password)
	usernameRow := formRow("Username", usernameEntry)
	passwordRow := formRow("Password", passwordEntry)

	updateAuthOptions := func() {
		if ikeVersionRadio.Selected == "IKEv1" {
			authModeSelect.Options = []string{"PSK only", "PSK + XAuth"}
			ikev1ModeRadio.Show()
		} else {
			authModeSelect.Options = []string{"PSK + EAP-MSCHAPv2"}
			ikev1ModeRadio.Hide()
		}
		authModeSelect.Refresh()
		if !containsStr(authModeSelect.Options, authModeSelect.Selected) {
			authModeSelect.SetSelected(authModeSelect.Options[0])
		}
	}
	updateCredentialRows := func() {
		if authModeSelect.Selected == "PSK + XAuth" || authModeSelect.Selected == "PSK + EAP-MSCHAPv2" {
			usernameRow.Show()
			passwordRow.Show()
		} else {
			usernameRow.Hide()
			passwordRow.Hide()
		}
	}
	ikeVersionRadio.OnChanged = func(string) { updateAuthOptions(); updateCredentialRows() }
	authModeSelect.OnChanged = func(string) { updateCredentialRows() }

	if p.IkeVersion == profile.IkeV1 {
		ikeVersionRadio.SetSelected("IKEv1")
	} else {
		ikeVersionRadio.SetSelected("IKEv2")
	}
	if p.Ikev1Mode == profile.Ikev1Aggressive {
		ikev1ModeRadio.SetSelected("Aggressive")
	} else {
		ikev1ModeRadio.SetSelected("Main")
	}
	updateAuthOptions()
	switch p.AuthMode {
	case profile.AuthPsk:
		authModeSelect.SetSelected("PSK only")
	case profile.AuthPskXauth:
		authModeSelect.SetSelected("PSK + XAuth")
	case profile.AuthEapMschapv2:
		authModeSelect.SetSelected("PSK + EAP-MSCHAPv2")
	}
	updateCredentialRows()

	splitRadio := widget.NewRadioGroup([]string{"Full tunnel (0.0.0.0/0)", "Custom routes"}, nil)
	cidrEntry := widget.NewMultiLineEntry()
	cidrEntry.SetPlaceHolder("10.0.0.0/8, 192.168.1.0/24")
	cidrEntry.SetText(strings.Join(p.SplitTunnel.CIDRs, ", "))
	if p.SplitTunnel.Kind == profile.SplitTunnelCustom {
		splitRadio.SetSelected("Custom routes")
	} else {
		splitRadio.SetSelected("Full tunnel (0.0.0.0/0)")
		cidrEntry.Hide()
	}
	splitRadio.OnChanged = func(sel string) {
		if sel == "Custom routes" {
			cidrEntry.Show()
		} else {
			cidrEntry.Hide()
		}
	}

	dpdEntry := widget.NewEntry()
	dpdEntry.SetText(p.DPDDelay)

	dhScroll := func(g *widget.CheckGroup) *container.Scroll {
		s := container.NewVScroll(g)
		s.SetMinSize(fyne.NewSize(420, 140))
		return s
	}

	form := container.NewVBox(
		formRow("Name", nameEntry),
		formRow("Gateway", gatewayEntry),
		formRow("Local ID", localIDEntry),
		formRow("Peer ID", remoteIDEntry),
		formRow("IKE Version", ikeVersionRadio),
		formRow("IKEv1 Mode", ikev1ModeRadio),
		formRow("Auth Mode", authModeSelect),
		formRow("Pre-Shared Key", pskEntry),
		usernameRow,
		passwordRow,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("IKE Proposal", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Ciphers (select at least one)"),
		ikeCipher,
		widget.NewLabel("Integrity (select at least one)"),
		ikeIntegrity,
		widget.NewLabel("DH Groups (select at least one)"),
		dhScroll(ikeDHGroups),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("ESP / Child SA Proposal", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Ciphers (select at least one)"),
		espCipher,
		widget.NewLabel("Integrity (select at least one)"),
		espIntegrity,
		widget.NewLabel("PFS DH Groups (optional)"),
		dhScroll(espDHGroups),
		widget.NewSeparator(),
		formRow("Split Tunnel", splitRadio),
		cidrEntry,
		formRow("DPD Delay", dpdEntry),
	)
	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(460, 480))

	title := "Add Profile"
	if !isNew {
		title = "Edit Profile"
	}

	d := dialog.NewCustomConfirm(title, "Save", "Cancel", scroll, func(confirmed bool) {
		if !confirmed {
			return
		}
		p.Name = nameEntry.Text
		p.Gateway = strings.TrimSpace(gatewayEntry.Text)
		p.LocalID = strings.TrimSpace(localIDEntry.Text)
		p.RemoteID = strings.TrimSpace(remoteIDEntry.Text)
		if ikeVersionRadio.Selected == "IKEv1" {
			p.IkeVersion = profile.IkeV1
			if ikev1ModeRadio.Selected == "Aggressive" {
				p.Ikev1Mode = profile.Ikev1Aggressive
			} else {
				p.Ikev1Mode = profile.Ikev1Main
			}
		} else {
			p.IkeVersion = profile.IkeV2
			p.Ikev1Mode = ""
		}
		switch authModeSelect.Selected {
		case "PSK only":
			p.AuthMode = profile.AuthPsk
		case "PSK + XAuth":
			p.AuthMode = profile.AuthPskXauth
		case "PSK + EAP-MSCHAPv2":
			p.AuthMode = profile.AuthEapMschapv2
		}

		ikeGroups := selectedKeywords(ikeDHGroups.Selected, dhByLabel)
		if len(ikeGroups) == 0 {
			dialog.ShowError(fmt.Errorf("select at least one DH group for the IKE proposal"), a.window)
			return
		}
		if len(ikeCipher.Selected) == 0 || len(ikeIntegrity.Selected) == 0 {
			dialog.ShowError(fmt.Errorf("select at least one cipher and one integrity algorithm for the IKE proposal"), a.window)
			return
		}
		if len(espCipher.Selected) == 0 || len(espIntegrity.Selected) == 0 {
			dialog.ShowError(fmt.Errorf("select at least one cipher and one integrity algorithm for the ESP proposal"), a.window)
			return
		}
		p.Proposals.IKE = buildProposals(ikeCipher.Selected, ikeIntegrity.Selected, ikeGroups)
		espGroups := selectedKeywords(espDHGroups.Selected, dhByLabel)
		p.Proposals.ESP = buildProposals(espCipher.Selected, espIntegrity.Selected, espGroups)

		if splitRadio.Selected == "Custom routes" {
			p.SplitTunnel = profile.SplitTunnel{Kind: profile.SplitTunnelCustom, CIDRs: splitCIDRs(cidrEntry.Text)}
		} else {
			p.SplitTunnel = profile.SplitTunnel{Kind: profile.SplitTunnelFull}
		}
		p.DPDDelay = dpdEntry.Text

		creds.PSK = pskEntry.Text
		creds.Username = usernameEntry.Text
		creds.Password = passwordEntry.Text

		a.saveProfile(p, creds, isNew)
	}, a.window)
	d.Resize(fyne.NewSize(500, 560))
	d.Show()
}

func formRow(label string, w fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, w)
}

func dhGroupOptions() (labels []string, byLabel map[string]string) {
	byLabel = make(map[string]string, len(profile.DHGroups))
	for _, g := range profile.DHGroups {
		labels = append(labels, g.Label)
		byLabel[g.Label] = g.Keyword
	}
	return labels, byLabel
}

func dhGroupLabelsForKeywords(keywords []string) []string {
	var labels []string
	for _, kw := range keywords {
		if g, ok := profile.DHGroupByKeyword(kw); ok {
			labels = append(labels, g.Label)
		}
	}
	return labels
}

// seedProposalPickers reverse-engineers cipher/integrity/DH-group selections
// from existing raw proposal strings ("aes256-sha256-modp2048") when editing
// a profile — cipher and integrity are assumed to be shared across all
// listed proposals for the same connection, which is the shape this GUI
// only ever produces.
// seedProposalPickers reverse-engineers cipher/integrity/DH-group
// selections from saved proposal strings. Every distinct component across
// the whole list is ticked, so a profile carrying two transforms round
// trips instead of losing all but the first.
func seedProposalPickers(proposals []string, cipher, integrity, dhGroups *widget.CheckGroup) {
	var ciphers, integrities, groups []string
	for _, prop := range proposals {
		parts := strings.Split(prop, "-")
		if len(parts) < 2 {
			continue
		}
		ciphers = appendUnique(ciphers, parts[0])
		integrities = appendUnique(integrities, parts[1])
		if len(parts) >= 3 {
			groups = append(groups, parts[2])
		}
	}
	if len(ciphers) > 0 {
		cipher.SetSelected(ciphers)
	}
	if len(integrities) > 0 {
		integrity.SetSelected(integrities)
	}
	if len(groups) > 0 {
		dhGroups.SetSelected(dhGroupLabelsForKeywords(groups))
	}
}

func horizontalCheckGroup(options []string) *widget.CheckGroup {
	g := widget.NewCheckGroup(options, nil)
	g.Horizontal = true
	return g
}

func defaultIfEmpty(g *widget.CheckGroup, value string) {
	if len(g.Selected) == 0 {
		g.SetSelected([]string{value})
	}
}

func appendUnique(list []string, v string) []string {
	if containsStr(list, v) {
		return list
	}
	return append(list, v)
}

func selectedKeywords(labels []string, byLabel map[string]string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if kw, ok := byLabel[l]; ok {
			out = append(out, kw)
		}
	}
	return out
}

// buildProposals expands the three checklists into one swanctl proposal
// string per cipher/integrity/DH-group combination. Several picker labels
// can map to the same strongSwan keyword (FortiGate group IDs vs. RFC
// names), so combinations are de-duplicated rather than repeated.
func buildProposals(ciphers, integrities, dhGroups []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(prop string) {
		if !seen[prop] {
			seen[prop] = true
			out = append(out, prop)
		}
	}
	for _, c := range ciphers {
		for _, i := range integrities {
			if len(dhGroups) == 0 {
				add(fmt.Sprintf("%s-%s", c, i))
				continue
			}
			for _, g := range dhGroups {
				add(fmt.Sprintf("%s-%s-%s", c, i, g))
			}
		}
	}
	return out
}

func splitCIDRs(text string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '\n' }) {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
