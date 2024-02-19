package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
)

type Config struct {
	Delay   int `toml:"delay"`
	SleepAt int `toml:"sleep_at"`
	WakeAt  int `toml:"wake_at"`
}

func readConfig() (*Config, error) {
	var cfg Config
	if _, err := os.Stat("config.toml"); os.IsNotExist(err) {
		// set default values
		cfg = Config{
			Delay:   190, // seconds
			SleepAt: 23,
			WakeAt:  6,
		}
		file, err := os.Create("config.toml")
		if err != nil {
			return nil, fmt.Errorf("failed to create config file: %v", err)
		}
		defer file.Close()

		// Write the default config to the file
		if err := toml.NewEncoder(file).Encode(cfg); err != nil {
			return nil, fmt.Errorf("failed to write default config to file: %v", err)
		}
	}

	// Load the config file
	var config Config
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		return nil, fmt.Errorf("failed to load config file: %v", err)
	}

	return &config, nil
}
