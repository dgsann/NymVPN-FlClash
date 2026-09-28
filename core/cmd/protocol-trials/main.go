package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/tunnel"
)

func run() error {
	dir := flag.String("d", ".", "state directory")
	file := flag.String("f", "", "private server config")
	check := flag.Bool("t", false, "validate config only")
	flag.Parse()
	C.SetHomeDir(*dir)
	cfg, err := executor.ParseWithPath(*file)
	if err != nil {
		return fmt.Errorf("server config rejected")
	}
	if len(cfg.Listeners) == 0 {
		return fmt.Errorf("server listeners missing")
	}
	if *check {
		return nil
	}
	executor.ApplyConfig(cfg, true)
	defer executor.Shutdown()
	// FlClash's embedded executor leaves listener lifecycle to its caller.
	for _, listener := range cfg.Listeners {
		if err := listener.Listen(tunnel.Tunnel); err != nil {
			return fmt.Errorf("listener %s failed: %w", listener.Name(), err)
		}
		defer listener.Close()
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	<-signals
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
