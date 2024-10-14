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

	switch cfg.Listen.Protocol {
	case "unix":
		listener, err = net.Listen("unix", cfg.Listen.Path)
	case "tcp":
		listenAddr := fmt.Sprintf("%s:%d", cfg.Listen.Address, cfg.Listen.Port)
		listener, err = net.Listen("tcp", listenAddr)
	default:
		log.Fatalf("Unsupported listen type: %s", cfg.Listen.Protocol)
	}

	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	targetURL, err := url.Parse(cfg.Proxy.Url)
	if err != nil {
		log.Fatalf("Invalid proxy URL: %v", err)
	}

	http.HandleFunc("/", proxy.ProxyHandler(cfg.Proxy.Protocol, targetURL, cfg.Proxy.UnixPath))

	server := &http.Server{
		ReadTimeout:  time.Duration(cfg.Proxy.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Proxy.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Proxy.IdleTimeout) * time.Second,
	}

	log.Printf("Listening on %s://%s", cfg.Listen.Protocol, listener.Addr())

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
