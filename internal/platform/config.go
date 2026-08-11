package platform

import (
	"os"
	"strings"
)

type Config struct {
	DatabaseURL      string
	RedisURL         string
	Port             string
	FirebaseCreds    string
	MapboxToken      string
	OverpassEndpointsRaw string
}

func LoadConfig() *Config {
	return &Config{
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"),
		RedisURL:             getEnv("REDIS_URL", "redis://localhost:6379"),
		Port:                 getEnv("PORT", "8080"),
		FirebaseCreds:        getEnv("FIREBASE_CREDENTIALS_FILE", "firebase-credentials.json"),
		MapboxToken:          getEnv("MAPBOX_TOKEN", ""),
		OverpassEndpointsRaw: getEnv("OVERPASS_ENDPOINTS", ""),
	}
}

// OverpassEndpoints returns the parsed Overpass URLs in preference order, or
// nil to use the NewOverpassClient built-in defaults. Env var format is
// comma-separated URLs: OVERPASS_ENDPOINTS=https://a/api,https://b/api
func (c *Config) OverpassEndpoints() []string {
	if c.OverpassEndpointsRaw == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(c.OverpassEndpointsRaw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
