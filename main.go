package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gen2brain/beeep"

	"YealinkNotifier/config"
	"YealinkNotifier/core"
	"YealinkNotifier/server"
	"YealinkNotifier/tray"
)

var version = "dev"

func setupLogging(logfile string, quiet bool) {
	if quiet {
		log.SetOutput(io.Discard)
		return
	}
	if logfile == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(logfile), 0o755); err != nil {
		log.Printf("Error creating log directory (%s): %v", filepath.Dir(logfile), err)
		return
	}
	f, err := os.OpenFile(logfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("Error opening log file (%s): %v", logfile, err)
		return
	}
	log.SetOutput(f)
}

func main() {
	port := flag.Int("port", config.DefaultPort, "working port")
	ip := flag.String("addr", config.DefaultAddr, "bind address")
	logfile := flag.String("logfile", config.DefaultLogfile(), "log file path (empty = write to the console)")
	quiet := flag.Bool("quiet", config.DefaultQuiet, "disable all logging")
	notify := flag.Bool("notify", true, "show system notification on incoming calls (disable = sound may still play)")
	beep := flag.Bool("beep", true, "play beep on incoming calls (disable = notification may still show)")
	allowedIPs := flag.String("allowed-ips", "", "comma-separated client IPs or CIDR networks allowed to call the webhook (e.g. 10.0.0.5,192.168.1.0/24; empty = allow all)")
	configPath := flag.String("config", "", "path to a YAML config file (empty = auto-discover: next to the binary, in the working directory, and in the user config dir)")
	versionFlag := flag.Bool("version", false, "show version")

	flag.Parse()
	if *versionFlag {
		fmt.Printf("version: %s\n", version)
		os.Exit(0)
	}

	explicit := make(map[string]bool)
	flag.Visit(func(fl *flag.Flag) { explicit[fl.Name] = true })

	eff, cfgPath, err := config.Load(*configPath, config.Flags{
		Port: *port, Addr: *ip, Notify: *notify, Beep: *beep,
		AllowedIPs: strings.Split(*allowedIPs, ","),
		Logfile:    *logfile, Quiet: *quiet,
		PortSet: explicit["port"], AddrSet: explicit["addr"],
		NotifySet: explicit["notify"], BeepSet: explicit["beep"],
		AllowedIPsSet: explicit["allowed-ips"],
		LogfileSet:    explicit["logfile"], QuietSet: explicit["quiet"],
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "WARN: no config file found; using built-in defaults")
	}

	if eff.Addr == "" || eff.Port == 0 {
		flag.Usage()
		os.Exit(1)
	}

	setupLogging(eff.Logfile, eff.Quiet)

	var allowed []string
	for _, p := range eff.AllowedIPs {
		if p = strings.TrimSpace(p); p != "" {
			allowed = append(allowed, p)
		}
	}

	beeep.AppName = "YealinkNotifier"

	addr := fmt.Sprintf("%s:%d", eff.Addr, eff.Port)

	if eff.Logfile != "" && !eff.Quiet {
		log.Printf("Logging to %s", eff.Logfile)
	}
	log.Printf("==================================================")
	log.Printf("   Version: %s", version)
	if cfgPath != "" {
		log.Printf("   Config file: %s", cfgPath)
	} else {
		log.Printf("   Config file: none (using defaults)")
	}
	log.Printf("   Server listening on port: %d", eff.Port)
	if len(allowed) > 0 {
		log.Printf("   Allowed client IPs: %d entries", len(allowed))
	}
	log.Printf("   URL for Yealink Webhook:")
	log.Printf("   http://%s/?number=$remote_number", addr)
	log.Printf("   or http://%s/?number=%%remote_number%%", addr)
	log.Printf("   or http://%s/", addr)
	log.Printf("==================================================")

	app := core.New(eff.Notify, eff.Beep)
	srv, err := server.New(app, addr, allowed)
	if err != nil {
		log.Printf("Error parsing allowed-ips (flag or config): %v", err)
		os.Exit(1)
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		app.Shutdown(ctx)
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		tray.Stop()
		shutdown()
	}()

	tray.RunSafe(app, shutdown)
}
