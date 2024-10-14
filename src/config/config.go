package config

import (
	"encoding/json"
	"os"
)

type ListenConfig struct {
	Address  string `json:"address"`
	Path     string `json:"path"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // "unix" or "tcp"
}

type ProxyConfig struct {
	ReadTimeout  int    `json:"readTimeout"`  // in seconds
	WriteTimeout int    `json:"writeTimeout"` // in seconds
	IdleTimeout  int    `json:"idleTimeout"`  // in seconds
	Protocol     string `json:"protocol"`     // "unix" or "tcp"
	UnixPath     string `json:"unixPath"`
	Url          string `json:"url"`
}

type Config struct {
	Listen ListenConfig `json:"listen"`
	Proxy  ProxyConfig  `json:"proxy"`
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
