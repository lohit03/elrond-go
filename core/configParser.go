package core

import (
	"github.com/ElrondNetwork/elrond-go/config"
	"github.com/ElrondNetwork/elrond-go/core/logger"
)

var log = logger.DefaultLogger()

// LoadP2PConfig returns a P2PConfig by reading the config file provided
func LoadP2PConfig(filepath string) (*config.P2PConfig, error) {
	cfg := &config.P2PConfig{}
	err := LoadTomlFile(cfg, filepath, log)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadServersPConfig returns a ServersConfig by reading the config file provided
func LoadServersPConfig(filepath string) (*config.ServersConfig, error) {
	cfg := &config.ServersConfig{}
	err := LoadTomlFile(cfg, filepath, log)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
