package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// browserLoginResult is what the local callback handler delivers back to the
// main login flow. Exactly one of Token or Err is non-zero.
type browserLoginResult struct {
	Token string
	Err   error
}

// browserLogin runs the full interactive flow:
//
//  1. Bind 127.0.0.1:0 for the callback server
//  2. Open https://<server>/dashboard/cli-auth?callback=...&state=...&name=...
//  3. Wait up to 5 minutes for the browser to redirect back with the token
//  4. Return the plaintext token (caller persists it)
//
// If the browser can't be opened (headless Linux, SSH, etc.), automatically
// falls back to manualTokenLogin which doesn't need a callback server — the
// user creates a token in the dashboard and pastes it at the CLI prompt.
//
// noBrowser=true forces the fallback path without even trying to open.
func browserLogin(server, name string, noBrowser bool, out io.Writer) (string, error) {
	// --no-browser: skip callback server entirely, go straight to manual flow
	if noBrowser {
		return manualTokenLogin(server, out)
	}

	state, err := randomState()
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen on localhost: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	callback := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	resultCh := make(chan browserLoginResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			resultCh <- browserLoginResult{Err: errors.New("state mismatch (possible CSRF) — abort")}
			return
		}
		if errVal := q.Get("error"); errVal != "" {
			writeCallbackHTML(w, false, "")
			resultCh <- browserLoginResult{Err: fmt.Errorf("authorization denied (%s)", errVal)}
			return
		}
		token := q.Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			resultCh <- browserLoginResult{Err: errors.New("callback returned no token")}
			return
		}
		writeCallbackHTML(w, true, "")
		resultCh <- browserLoginResult{Token: token}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	authURL := buildAuthURL(server, callback, state, name)

	fmt.Fprintf(out, "Opening %s in your browser...\n", authURL)
	if err := openBrowser(authURL); err != nil {
		// Browser can't open — shut down the callback server and fall back
		// to the manual token paste flow. This handles headless Linux, SSH
		// to a remote server, Docker containers, etc.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		_ = listener.Close()

		fmt.Fprintf(out, "\nCould not open a browser (%v).\n", err)
		fmt.Fprintln(out, "Falling back to manual token entry.\n")
		return manualTokenLogin(server, out)
	}

	fmt.Fprintln(out, "Waiting for authorization... (Ctrl+C to abort)")

	// Honour Ctrl+C while waiting.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case res := <-resultCh:
		if res.Err != nil {
			return "", res.Err
		}
		return res.Token, nil
	case <-sigCh:
		return "", errors.New("aborted by user")
	case <-ctx.Done():
		return "", errors.New("authorization timed out after 5 minutes")
	}
}

// manualTokenLogin is the no-callback fallback. It directs the user to the
// dashboard's API Tokens page to create a token, then reads it from stdin.
//
// This works in every environment — SSH, headless Docker, screen, tmux — as
// long as the user has a browser somewhere (phone, another laptop, etc.) and
// can copy-paste.
func manualTokenLogin(server string, out io.Writer) (string, error) {
	name := defaultTokenName()
	q := url.Values{}
	q.Set("name", name)
	q.Set("mode", "manual")
	tokenURL := strings.TrimRight(server, "/") + "/dashboard/cli-auth?" + q.Encode()

	fmt.Fprintln(out, "Open this URL in any browser to authorize the CLI:")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s\n", tokenURL)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "After authorizing, copy the token and paste it below.")
	fmt.Fprint(out, "Token: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", errors.New("no input received")
	}
	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return "", errors.New("empty token")
	}
	return token, nil
}

// buildAuthURL constructs the cli-auth URL the browser should land on. The
// path is `/dashboard/cli-auth` because the React SPA is mounted under that
// base path in production.
func buildAuthURL(server, callback, state, name string) string {
	base := strings.TrimRight(server, "/")
	q := url.Values{}
	q.Set("callback", callback)
	q.Set("state", state)
	q.Set("name", name)
	return fmt.Sprintf("%s/dashboard/cli-auth?%s", base, q.Encode())
}

// randomState returns 32 bytes of crypto/rand encoded as URL-safe base64
// (43 chars, 256 bits of entropy). Used as the CSRF state nonce.
func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// openBrowser opens url in the OS default browser. Falls through to error on
// unsupported platforms; caller is expected to fall back to manual flow.
func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "linux":
		// Headless Linux (SSH, Docker, no desktop) — xdg-open will start
		// but silently fail; .Start() returns nil so fallback never fires.
		// Guard by requiring DISPLAY or WAYLAND_DISPLAY to be set.
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return fmt.Errorf("no display server (headless)")
		}
		return exec.Command("xdg-open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	}
	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

// writeCallbackHTML writes a minimal "you can close this tab" page back to
// the browser. No JS, no external resources — safe with strict CSPs.
func writeCallbackHTML(w http.ResponseWriter, success bool, _ string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if success {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<!doctype html><html><head><meta charset="utf-8"><title>Liaison CLI</title>
<style>body{font-family:-apple-system,sans-serif;text-align:center;padding:6em 2em;color:#1f2937}h1{color:#16a34a}</style>
</head><body><h1>✓ Authorized</h1><p>You can close this tab and return to your terminal.</p></body></html>`)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!doctype html><html><head><meta charset="utf-8"><title>Liaison CLI</title>
<style>body{font-family:-apple-system,sans-serif;text-align:center;padding:6em 2em;color:#1f2937}h1{color:#dc2626}</style>
</head><body><h1>✗ Denied</h1><p>The CLI did not receive a token. You can close this tab.</p></body></html>`)
}

// defaultTokenName generates a sensible name for tokens minted via the
// browser flow: `cli-<host>-<yyyymmdd>`. Users can override with --name.
func defaultTokenName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	host = strings.ReplaceAll(host, ".", "-")
	if len(host) > 32 {
		host = host[:32]
	}
	return fmt.Sprintf("cli-%s-%s", host, time.Now().Format("20060102"))
}
