package saml

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLoginFlow(t *testing.T) {
	var gotUID string
	gw := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/saml_login" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.HasPrefix(string(raw), `{"type": "rawData", "bytes": "UID=`) {
			t.Errorf("body must keep FortiClient's field order, got %s", raw)
		}
		var body map[string]string
		_ = json.Unmarshal(raw, &body)
		v, _ := url.ParseQuery(body["bytes"])
		gotUID = v.Get("UID")
		if v.Get("REDIRECT_PORT") != strconv.Itoa(RedirectPort) || body["type"] != "rawData" {
			t.Errorf("unexpected body %v", body)
		}
		io.WriteString(w, `<html><body><script language="JavaScript">window.location="https://gw.example.test:9443/saml?0a1b";</script></body></html>`)
	}))
	defer gw.Close()
	u, _ := url.Parse(gw.URL)
	port, _ := strconv.Atoi(u.Port())

	s, err := Begin(context.Background(), gw.Client(), u.Hostname(), port)
	if err != nil {
		t.Fatal(err)
	}
	if s.LoginURL != "https://gw.example.test:9443/saml?0a1b" || len(s.UID) != 32 || s.UID != strings.ToUpper(s.UID) || gotUID != s.UID {
		t.Fatalf("bad session: %+v (gateway saw UID %q)", s, gotUID)
	}

	// The browser coming back with the token.
	go func() {
		time.Sleep(100 * time.Millisecond)
		resp, err := http.Get("http://127.0.0.1:8020/?tokenid=ABCDEF0123&username=user@example.test")
		if err == nil {
			resp.Body.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := s.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.UID != s.UID || r.Token != "ABCDEF0123" || r.Username != "user@example.test" {
		t.Fatalf("bad result %+v", r)
	}
}

func TestGatewayWithoutSAML(t *testing.T) {
	gw := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html>login</html>")
	}))
	defer gw.Close()
	u, _ := url.Parse(gw.URL)
	port, _ := strconv.Atoi(u.Port())
	if _, err := Begin(context.Background(), gw.Client(), u.Hostname(), port); err == nil || !strings.Contains(err.Error(), "did not return a login URL") {
		t.Fatalf("expected a clear error, got %v", err)
	}
	// The listener must have been released.
	s, err := Begin(context.Background(), gw.Client(), u.Hostname(), port)
	if err == nil {
		s.Close()
	}
	if err != nil && strings.Contains(err.Error(), "busy") {
		t.Fatal("listener leaked after a failed Begin")
	}
}
