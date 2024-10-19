package config

import (
	"encoding/json"
	"os"
)

type ServerConfig struct {
	Address      string `json:"address"`
	Path         string `json:"path"`
	Port         int    `json:"port"`
	Protocol     string `json:"protocol"`     // "unix" or "tcp"
	ReadTimeout  int    `json:"readTimeout"`  // in seconds
	WriteTimeout int    `json:"writeTimeout"` // in seconds
	IdleTimeout  int    `json:"idleTimeout"`  // in seconds
}

type ProxyConfig struct {
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

type LimitConfig struct {
	Cc       int      `json:"cc"`
	Mentions int      `json:"mentions"`
	Keywords []string `json:"keywords"`
}

type Config struct {
	Server ServerConfig `json:"server"`
	Proxy  ProxyConfig  `json:"proxy"`
	Limit  LimitConfig  `json:"limit"`
}

func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := &Config{}
	err = json.NewDecoder(file).Decode(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
