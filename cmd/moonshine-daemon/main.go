package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"moonshine-daemon/internal/config"
	"moonshine-daemon/internal/daemon"
	"moonshine-daemon/internal/moonshine"
	"moonshine-daemon/internal/tray"
)

func main() {
	configPath := flag.String("config", config.DefaultPath, "config file path")
	verbose := flag.Bool("verbose", false, "verbose logging")
	noTray := flag.Bool("no-tray", false, "disable system tray icon")
	flag.Parse()

	log.SetPrefix("moonshine: ")
	log.SetFlags(log.Ltime)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %s", err)
	}

	// Determine model path and architecture
	modelPath := resolveModelPath(cfg.Language())
	arch := moonshine.ArchMediumStreaming
	if *verbose {
		log.Printf("model: %s (arch: medium-streaming)", modelPath)
	}

	// Load transcriber (model stays loaded for entire daemon lifetime)
	log.Println("loading model...")
	transcriber, err := moonshine.NewTranscriber(modelPath, arch)
	if err != nil {
		log.Fatalf("load transcriber: %s", err)
	}
	log.Println("model loaded")

	// Sound directory
	soundDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "moonshine", "sounds")

	// Create daemon
	d := daemon.New(transcriber, cfg, soundDir, *verbose)

	// Start socket server
	sock, err := daemon.NewSocketServer(d, *verbose)
	if err != nil {
		log.Fatalf("socket server: %s", err)
	}

	// Shutdown function
	shutdown := func() {
		log.Println("shutting down...")
		sock.Close()
		d.Close()
	}

	// Graceful shutdown on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("moonshine-daemon running")
	fmt.Printf("  socket: %s\n", daemon.SocketPath)
	if !*noTray {
		fmt.Println("  tray: enabled")
	}

	if !*noTray {
		// Start tray in background — if it exits (D-Bus flake), daemon keeps running
		go func() {
			log.Println("starting system tray...")
			tray.Run(d, *verbose)
			log.Println("system tray exited (daemon continues on socket)")
		}()
	}

	// Block until signal or quit command
	select {
	case <-sigCh:
	case <-sock.QuitCh:
	}
	shutdown()
}

func resolveModelPath(language string) string {
	home := os.Getenv("HOME")
	cacheDir := filepath.Join(home, ".cache", "moonshine_voice", "download.moonshine.ai", "model")

	modelName := "medium-streaming-en"
	switch language {
	case "es":
		modelName = "base-es"
	case "ar":
		modelName = "base-ar"
	case "ja":
		modelName = "base-ja"
	}

	path := filepath.Join(cacheDir, modelName, "quantized")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	path = filepath.Join(cacheDir, modelName)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	log.Fatalf("model not found at %s — run moonshine once with Python to download it", path)
	return ""
}
