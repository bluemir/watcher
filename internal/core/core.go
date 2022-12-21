package core

import "context"

func NewConfig() *Config {
	return &Config{}
}

type Config struct {
	DryRun bool
}

func Run(ctx context.Context, conf *Config) error {

	return nil
}
