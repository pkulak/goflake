package main

import (
	"github.com/BurntSushi/toml"
	"io/ioutil"
)

const (
	ClusterStateNew      = "new"
	ClusterStateExisting = "existing"
)

type RaftConfig struct {
	Addr         string   `toml:"addr"`
	DataDir      string   `toml:"data_dir"`
	Cluster      []string `toml:"cluster"`
	ClusterState string   `toml:"cluster_state"`
}

type Config struct {
	Raft RaftConfig `toml:"raft"`
	Addr string     `toml:"addr"`
}

func NewConfigWithFile(name string) (*Config, error) {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}

	return NewConfig(string(data))
}

func NewConfig(data string) (*Config, error) {
	var c Config

	_, err := toml.Decode(data, &c)
	if err != nil {
		return nil, err
	}

	return &c, nil
}
