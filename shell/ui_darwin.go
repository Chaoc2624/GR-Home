//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"grmod/shell/internal/platform"
)

// runPlatform is the macOS mode: like the headless mode, but the default
// browser is opened on the UI automatically. The server's heartbeat stops
// the process once the page is gone, exactly as with the Windows
// --browser fallback.
func runPlatform(a *app) int {
	if err := a.start(platform.New()); err != nil {
		a.log.Printf("startup failed: %v", err)
		fatalMessage("GR Mod cannot start: " + err.Error())
		return 1
	}
	fmt.Printf("LISTEN %s TOKEN %s\n", a.url, a.token)
	// Best effort: if the browser cannot be opened the address above is
	// still printed, so the UI can be opened by hand.
	_ = exec.Command("open", a.url).Start()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	sig := <-stop
	a.log.Printf("received %v", sig)
	a.shutdown()
	return 0
}

func fatalMessage(msg string) {
	fmt.Fprintln(os.Stderr, "grmod: "+msg)
}
