package cli

import (
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
// noBrowser=true skips the auto-open and prints the URL instead.
func browserLogin(server, name string, noBrowser bool, out io.Writer) (string, error) {
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

	if noBrowser {
		fmt.Fprintf(out, "Open this URL in a browser to authorize the CLI:\n\n  %s\n\nWaiting for callback...\n", authURL)
	} else {
		fmt.Fprintf(out, "Opening %s in your browser...\n", authURL)
		if err := openBrowser(authURL); err != nil {
			fmt.Fprintf(out, "Could not auto-open browser (%v).\nPlease open this URL manually:\n  %s\n\nWaiting for callback...\n", err, authURL)
		} else {
			fmt.Fprintln(out, "Waiting for authorization... (Ctrl+C to abort)")
		}
	}

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
// unsupported platforms; caller is expected to print the URL manually.
func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "linux":
		return exec.Command("xdg-open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	}
	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

// writeCallbackHTML writes a minimal "you can close this tab" page back to
// the browser. We keep it small and dependency-free; no JS, no external
// resources, so it's safe to render even with strict CSPs.
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
