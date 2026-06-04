//go:build desktop

// Command my-orchestra-desktop wraps the my-orchestra web UI in a native
// desktop window via Wails v3. It serves the same HTTP handler on a random
// loopback port (bootstrap.ServeLocal) and points an OS webview (WebView2 on
// Windows, WKWebView on macOS, WebKitGTK on Linux) at that URL — so the htmx +
// SSE UI is byte-identical to the browser experience and SSE flows natively,
// bypassing Wails' wails:// asset protocol. See docs/adr/0006-*.
//
// This package is isolated behind the `desktop` build tag because Wails pulls
// in CGO and platform webview headers; the default `go build/test ./...` (and
// the pure-Go web binary cmd/my-orchestra) never compile it. Build with:
//
//	go build -tags desktop -o bin/my-orchestra-desktop ./cmd/my-orchestra-desktop
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/dotts-h/copilot-sdk/internal/bootstrap"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	var (
		showVersion bool
		configDir   string
		demo        bool
		serve       bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&configDir, "config-dir", bootstrap.DefaultConfigDir(), "configuration directory")
	flag.BoolVar(&demo, "demo", false, "drive the UI with a scripted mock (no Copilot runtime)")
	flag.BoolVar(&serve, "serve", false, "run the HTTP server without opening a window (headless; for CI/smoke and Playwright)")
	flag.Parse()

	if showVersion {
		fmt.Printf("my-orchestra-desktop %s\n", version)
		return
	}

	if err := run(configDir, demo, serve); err != nil {
		fmt.Fprintln(os.Stderr, "my-orchestra-desktop: "+err.Error())
		os.Exit(1)
	}
}

// run assembles the shared server, serves it on a loopback port, and either
// opens a native window pointed at it or (with -serve) blocks headless until
// interrupted. Teardown is idempotent so the defer and Wails' OnShutdown can
// both fire without double-closing the client.
func run(configDir string, demo, serve bool) error {
	srv, closeFn, err := bootstrap.Build(configDir, demo)
	if err != nil {
		return err
	}

	port, stop, err := bootstrap.ServeLocal(srv.Handler())
	if err != nil {
		closeFn()
		return err
	}

	var once sync.Once
	shutdown := func() { once.Do(func() { stop(); closeFn() }) }
	defer shutdown()

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)

	if serve {
		// Headless: serve until interrupted. Lets CI smoke-test the boot path
		// without a display, and lets Playwright target the desktop binary.
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		fmt.Printf("my-orchestra-desktop serving headless on %s\n", url)
		<-ctx.Done()
		return nil
	}

	app := application.New(application.Options{
		Name: "my-orchestra",
		// We navigate to an external loopback URL, so the asset server is never
		// hit; a 404 handler satisfies Wails without embedding assets.
		Assets:     application.AssetOptions{Handler: http.NotFoundHandler()},
		OnShutdown: shutdown,
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "my-orchestra",
		Width:  1200,
		Height: 800,
		URL:    url,
	})
	return app.Run()
}
