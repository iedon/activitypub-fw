package config

import (
	"encoding/json"
	"os"
	"sync"
)

type serverConfig struct {
	Debug                bool     `json:"debug"` // Will print detail access log for debug
	Address              string   `json:"address"`
	Path                 string   `json:"path"`
	Port                 int      `json:"port"`
	Protocol             string   `json:"protocol"`             // "unix" or "tcp"
	ReadTimeout          int      `json:"readTimeout"`          // in seconds
	WriteTimeout         int      `json:"writeTimeout"`         // in seconds
	IdleTimeout          int      `json:"idleTimeout"`          // in seconds
	InboundProxyNetworks []string `json:"inboundProxyNetworks"` // String array of IP networks in CIDR.
	// X-Forwarded headers from these networks will be trusted.
}

type proxyConfig struct {
	Protocol              string `json:"protocol"` // "unix" or "tcp"
	UnixPath              string `json:"unixPath"`
	Url                   string `json:"url"`
	ForceAttemptHTTP2     bool   `json:"forceAttemptHttp2"`
	KeepAlive             int    `json:"keepAlive"`
	Timeout               int    `json:"timeout"`
	MaxIdleConns          int    `json:"maxIdleConns"`
	MaxIdleConnsPerHost   int    `json:"maxIdleConnsPerHost"`
	MaxConnsPerHost       int    `json:"maxConnsPerHost"`
	IdleConnTimeout       int    `json:"idleConnTimeout"`
	TLSHandshakeTimeout   int    `json:"tlsHandshakeTimeout"`
	ExpectContinueTimeout int    `json:"expectContinueTimeout"`
	WriteBufferSize       int    `json:"writeBufferSize"`
	ReadBufferSize        int    `json:"readBufferSize"`
}

type limitConfig struct {
	MaxBodySize int64    `json:"maxBodySize"`
	Cc          int      `json:"cc"`
	Mentions    int      `json:"mentions"`
	Keywords    []string `json:"keywords"`
}

type appConfig struct {
	Server serverConfig `json:"server"`
	Proxy  proxyConfig  `json:"proxy"`
	Limit  limitConfig  `json:"limit"`
}

type Config struct {
	sync.RWMutex
	Config *appConfig
}

func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := &Config{}
	cfg.Config = &appConfig{}

	err = json.NewDecoder(file).Decode(cfg.Config)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
