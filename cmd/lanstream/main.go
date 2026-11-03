// Command lanstream streams the videos in a folder to VLC on the local network.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// After the first Ctrl-C, restore the default handling so a second one ends the process
	// immediately instead of waiting out the shutdown.
	go func() {
		<-ctx.Done()
		stop()
	}()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
