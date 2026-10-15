// Package saml implements FortiGate's browser-based SAML login for dial-up
// IPsec, as FortiClient does it:
//
//  1. listen on 127.0.0.1:<RedirectPort> (FortiClient uses 8020);
//  2. POST https://<gateway>:<saml port>/saml_login? with a random 32-hex UID
//     and the redirect port; the reply is a tiny HTML page whose
//     window.location is the FortiGate's /saml?<id> URL;
//  3. open that URL in the user's browser: the FortiGate forwards it to the
//     IdP (Entra ID, Okta, FortiAuthenticator...), the user logs in;
//  4. the FortiGate redirects the browser to
//     http://127.0.0.1:<RedirectPort>/?tokenid=<token>&username=<user>.
//
// The tunnel is then authenticated with UID as the identity and the token as
// the secret: XAuth on IKEv1, EAP on IKEv2. The token is single-use.
//
// TLS to the SAML port is always verified; Host must be the name on the
// gateway's certificate (not the IP the profile may pin).
package saml

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// RedirectPort is where FortiClient (and therefore FortiGate configs in the
// wild) expect the browser to come back.
const RedirectPort = 8020

// Result is what the browser login yields.
type Result struct {
	UID      string // identity to present in XAuth/EAP
	Token    string // one-time secret
	Username string // the IdP account that logged in (informational)
}

// Session is one login attempt in progress.
type Session struct {
	UID      string
	LoginURL string // open this in the browser
	listener net.Listener
	server   *http.Server
	result   chan Result
}

var locationRE = regexp.MustCompile(`window\.location\s*=\s*"([^"]+)"`)

// Begin starts the local listener and asks the FortiGate for the login URL.
// client may be nil (http.DefaultClient with verified TLS).
func Begin(ctx context.Context, client *http.Client, host string, port int) (*Session, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	uid, err := newUID()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", RedirectPort))
	if err != nil {
		return nil, fmt.Errorf("local port %d is busy (another login, or FortiClient running?): %w", RedirectPort, err)
	}
	s := &Session{UID: uid, listener: ln, result: make(chan Result, 1)}

	// Byte-for-byte the body that worked against a real FortiGate: "type"
	// first, then "bytes". json.Marshal of a map sorts keys ("bytes" first);
	// the gateway still answers with a login URL but the resulting token was
	// then rejected in XAuth — the field order matters to its parser.
	body := []byte(fmt.Sprintf(`{"type": "rawData", "bytes": "UID=%s&REDIRECT_PORT=%d"}`, uid, RedirectPort))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s:%d/saml_login?", host, port), bytes.NewReader(body))
	if err != nil {
		ln.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("SAML service at %s:%d: %w", host, port, err)
	}
	defer resp.Body.Close()
	page, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	m := locationRE.FindSubmatch(page)
	if resp.StatusCode != http.StatusOK || m == nil {
		ln.Close()
		return nil, fmt.Errorf("SAML service at %s:%d did not return a login URL (HTTP %d) — is SAML enabled on this gateway?", host, port, resp.StatusCode)
	}
	s.LoginURL = string(m[1])
	if !strings.HasPrefix(s.LoginURL, "https://") {
		ln.Close()
		return nil, fmt.Errorf("unexpected login URL from gateway: %q", s.LoginURL)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		token := q.Get("tokenid")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if token == "" {
			http.Error(w, "missing tokenid", http.StatusBadRequest)
			return
		}
		io.WriteString(w, "<h3>Login received. You can close this tab and return to FortiVPN.</h3>")
		select {
		case s.result <- Result{UID: uid, Token: token, Username: q.Get("username")}:
		default: // a second callback for the same session is ignored
		}
	})
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go s.server.Serve(ln)
	return s, nil
}

// Wait blocks until the browser comes back, ctx ends, or Close is called.
func (s *Session) Wait(ctx context.Context) (Result, error) {
	defer s.Close()
	select {
	case r := <-s.result:
		return r, nil
	case <-ctx.Done():
		return Result{}, errors.New("SAML login was not completed in time")
	}
}

// Close stops the local listener.
func (s *Session) Close() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func newUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}
