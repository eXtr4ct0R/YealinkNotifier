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
	"syscall"
	"time"

	"github.com/gen2brain/beeep"

	"YealinkNotifier/core"
	"YealinkNotifier/server"
	"YealinkNotifier/tray"
)

var version = "dev"

func defaultLogPath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "YealinkNotifier", "yealinknotifier.log")
}

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
	port := flag.Int("port", 8080, "working port")
	ip := flag.String("addr", "0.0.0.0", "bind address")
	logfile := flag.String("logfile", defaultLogPath(), "log file path (empty = write to the console)")
	quiet := flag.Bool("quiet", false, "disable all logging")
	versionFlag := flag.Bool("version", false, "show version")

	flag.Parse()
	if *versionFlag {
		fmt.Printf("version: %s\n", version)
		os.Exit(0)
	}

	if *ip == "" || *port == 0 {
		flag.Usage()
		os.Exit(1)
	}

	setupLogging(*logfile, *quiet)

	beeep.AppName = "YealinkNotifier"

	addr := fmt.Sprintf("%s:%d", *ip, *port)

	if *logfile != "" && !*quiet {
		log.Printf("Logging to %s", *logfile)
	}
	log.Printf("==================================================")
	log.Printf("   Version: %s", version)
	log.Printf("   Server listening on port: %d", *port)
	log.Printf("   URL for Yealink Webhook:")
	log.Printf("   http://%s/?number=$remote_number", addr)
	log.Printf("   or http://%s/?number=%%remote_number%%", addr)
	log.Printf("   or http://%s/", addr)
	log.Printf("==================================================")

	app := core.New()
	srv := server.New(app, addr)

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
