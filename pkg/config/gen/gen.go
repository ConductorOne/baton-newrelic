package main

import (
	cfg "github.com/conductorone/baton-newrelic/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/config"
)

func main() {
	config.Generate("newrelic", cfg.Config)
}
