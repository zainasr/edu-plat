// Package config reads settings from environment variables (repo-layout: config comes
// from the environment, never from committed files). Go does not read .env by itself.
package config

import (
	"errors"
	"fmt"
	"os"
)

// Config is everything the API needs at start-up.
type Config struct {
	HTTPAddr    string // HTTP_ADDR, default :8080
	Env         string // APP_ENV: "dev" or "production"; default "production" (safe)
	DatabaseURL string // DATABASE_URL: the non-owner API role (app_rt)
	AuthSecret  []byte // AUTH_SECRET: HMAC key for OTP codes; at least 32 bytes
}

// IsDev is true only when APP_ENV is exactly "dev". Dev-only behaviour (a logging OTP sender)
// depends on this, so an unset variable is never dev.
func (c Config) IsDev() bool { return c.Env == "dev" }

// Load reads and validates the environment. It returns every problem at once.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		Env:         getenv("APP_ENV", "production"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		AuthSecret:  []byte(os.Getenv("AUTH_SECRET")),
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.AuthSecret) < 32 {
		errs = append(errs, fmt.Errorf("AUTH_SECRET must be at least 32 bytes (got %d)", len(c.AuthSecret)))
	}
	if c.Env != "dev" && c.Env != "production" {
		errs = append(errs, fmt.Errorf("APP_ENV must be dev or production (got %q)", c.Env))
	}
	return c, errors.Join(errs...)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
