package ydls

import (
	"encoding/json"
	"io"
)

// YDLS config
type Config struct {
	InputFlags []string
	Formats    Formats
}

func parseConfig(r io.Reader) (Config, error) {
	c := Config{}

	d := json.NewDecoder(r)
	if err := d.Decode(&c); err != nil {
		return Config{}, err
	}

	return c, nil
}
