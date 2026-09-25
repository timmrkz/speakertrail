// Package config reads secrets and deployment settings from environment
// variables. Everything Tim tunes lives in the settings table instead.
package config

import (
	"errors"
	"fmt"
	"os"
)

// Config holds the environment of one process.
type Config struct {
	DatabaseURL     string
	TavilyAPIKey    string
	AnthropicAPIKey string
	UIPasswordHash  string
	SessionSecret   string
	// Port is where serve listens. Default 8080.
	Port string
}

// FromEnv reads the configuration from the environment.
func FromEnv() Config {
	c := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		TavilyAPIKey:    os.Getenv("TAVILY_API_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		UIPasswordHash:  os.Getenv("UI_PASSWORD_HASH"),
		SessionSecret:   os.Getenv("SESSION_SECRET"),
		Port:            os.Getenv("PORT"),
	}
	if c.Port == "" {
		c.Port = "8080"
	}
	return c
}

// Require returns an error naming every listed variable that is empty.
func (c Config) Require(names ...string) error {
	values := map[string]string{
		"DATABASE_URL":      c.DatabaseURL,
		"TAVILY_API_KEY":    c.TavilyAPIKey,
		"ANTHROPIC_API_KEY": c.AnthropicAPIKey,
		"UI_PASSWORD_HASH":  c.UIPasswordHash,
		"SESSION_SECRET":    c.SessionSecret,
	}
	var errs []error
	for _, n := range names {
		v, known := values[n]
		if !known {
			errs = append(errs, fmt.Errorf("unknown variable %s", n))
		} else if v == "" {
			errs = append(errs, fmt.Errorf("%s is not set", n))
		}
	}
	return errors.Join(errs...)
}
