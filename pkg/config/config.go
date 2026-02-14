package config

import (
	"fmt"

	"github.com/conductorone/baton-sdk/pkg/field"
)

var (
	Apikey = field.StringField(
		"apikey",
		field.WithDescription("The API key used to connect to NewRelic GraphQL API"),
		field.WithRequired(true),
		field.WithIsSecret(true),
		field.WithDisplayName("API Key"),
	)

	BaseURLField = field.StringField(
		"base-url",
		field.WithDescription("Override the NewRelic API URL (for testing)"),
		field.WithHidden(true),
	)

	// FieldRelationships defines relationships between the fields listed in
	// Config that can be automatically validated.
	FieldRelationships = []field.SchemaFieldRelationship{}
)

//go:generate go run ./gen
var Config = field.NewConfiguration([]field.SchemaField{
	Apikey,
	BaseURLField,
})

// ValidateConfig is run after the configuration is loaded, and should return an
// error if it isn't valid. Implementing this function is optional, it only
// needs to perform extra validations that cannot be encoded with configuration
// parameters.
func ValidateConfig(cfg *Newrelic) error {
	if cfg.Apikey == "" {
		return fmt.Errorf("apikey must be provided")
	}
	return nil
}
