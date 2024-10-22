package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/iedon/activitypub-fw/config"
	"github.com/iedon/activitypub-fw/proxy"
)

var cfg *config.Config
var server *http.Server

func main() {
	configFile := flag.String("c", "config.json", "Path to the JSON configuration file")
	help := flag.Bool("h", false, "Print this message")
	flag.Parse()

	if *help {
		fmt.Fprintln(os.Stderr, "Usage:", os.Args[0], "[-c config_file]")
		flag.PrintDefaults()
		return
	}

	loadConfig(*configFile)

	listener, err := createListener(cfg)
	if err != nil {
		log.Fatalf("Failed to listen: %v\n", err)
	}

	server = &http.Server{}
	setServerParameters(server, cfg)

	http.HandleFunc("/", proxy.ProxyHandler(cfg))

	log.Printf("Listening on %s://%s\n", cfg.Config.Server.Protocol, listener.Addr())
	daemon(listener)
}

func loadConfig(configFile string) {
	var err error
	cfg, err = config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v\n", err)
	}

	go watchConfig(configFile)
}

// CreateListener initializes a net.Listener based on the server configuration and environment
func createListener(cfg *config.Config) (net.Listener, error) {
	var listener net.Listener
	var err error

	if os.Getenv("LISTEN_PID") == strconv.Itoa(os.Getpid()) {
		// Run from systemd
		const SD_LISTEN_FDS_START = 3
		f := os.NewFile(SD_LISTEN_FDS_START, "")
		listener, err = net.FileListener(f)
	} else {
		switch strings.ToLower(cfg.Config.Server.Protocol) {
		case "unix":
			listener, err = net.Listen("unix", cfg.Config.Server.Path)
		case "tcp":
			listenAddr := fmt.Sprintf("%s:%d", cfg.Config.Server.Address, cfg.Config.Server.Port)
			listener, err = net.Listen("tcp", listenAddr)
		default:
			return nil, fmt.Errorf("unsupported listen type: %s", cfg.Config.Server.Protocol)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	return listener, nil
}

func setServerParameters(s *http.Server, cfg *config.Config) {
	if s == nil {
		return
	}

	s.ReadTimeout = time.Duration(cfg.Config.Server.ReadTimeout) * time.Second
	s.WriteTimeout = time.Duration(cfg.Config.Server.WriteTimeout) * time.Second
	s.IdleTimeout = time.Duration(cfg.Config.Server.IdleTimeout) * time.Second
}

// Use as a Goroutine to handle server shutdown gracefully
func daemon(listener net.Listener) {
	defer listener.Close()

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
		log.Fatalln("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

// watchConfig sets up a watcher on the configuration file to reload it on changes
// This is to reload when limit rules changed, we do not care about other changes for now
func watchConfig(filename string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println(err)
	}
	defer watcher.Close()

	absPath, err := filepath.Abs(filename)
	if err != nil {
		log.Fatalln(err)
	}

	err = watcher.Add(filepath.Dir(absPath))
	if err != nil {
		log.Fatalln(err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) && event.Name == absPath {
				newCfg, err := config.LoadConfig(filename)
				if err != nil {
					log.Printf("Error reloading config: %v\n", err)
					return
				}

				cfg.Lock()
				cfg.Config = newCfg.Config
				cfg.Unlock()

				setServerParameters(server, cfg)

				log.Println("Config reloaded successfully")
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				log.Println(err)
				return
			}
			log.Printf("Watcher error: %v\n", err)
		}
	}
}
