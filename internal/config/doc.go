// Package config loads application configuration with a four-layer
// priority system: CLI flags > environment variables > YAML config file >
// compiled defaults.
//
// Use [Load] to build an [AppConfig] from os.Args.
package config
