package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/iedon/activitypub-fw/config"
	"github.com/iedon/activitypub-fw/proxy"
)

func main() {
	configFilePath := flag.String("c", "config.json", "Path to the JSON configuration file")
	help := flag.Bool("h", false, "Print this message")
	flag.Parse()

	if *help {
		fmt.Fprintln(os.Stderr, "Usage:", os.Args[0], "[-c config_file]")
		flag.PrintDefaults()
		os.Exit(0)
	}

	cfg, err := config.LoadConfig(*configFilePath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	var listener net.Listener

	if os.Getenv("LISTEN_PID") == strconv.Itoa(os.Getpid()) {
		// Run from systemd
		const SD_LISTEN_FDS_START = 3
		f := os.NewFile(SD_LISTEN_FDS_START, "")
		listener, err = net.FileListener(f)
	} else {
		switch cfg.Server.Protocol {
		case "unix":
			listener, err = net.Listen("unix", cfg.Server.Path)
		case "tcp":
			listenAddr := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
			listener, err = net.Listen("tcp", listenAddr)
		default:
			log.Fatalf("Unsupported listen type: %s", cfg.Server.Protocol)
		}
	}

	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	targetURL, err := url.Parse(cfg.Proxy.Url)
	if err != nil {
		log.Fatalf("Invalid proxy URL: %v", err)
	}

	http.HandleFunc("/", proxy.ProxyHandler(targetURL, &cfg.Proxy))

	server := &http.Server{
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	log.Printf("Listening on %s://%s", cfg.Server.Protocol, listener.Addr())

	// Use a Goroutine to handle server shutdown gracefully
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not serve: %v\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
