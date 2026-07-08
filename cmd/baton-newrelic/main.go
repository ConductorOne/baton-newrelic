package main

import (
	"context"

	cfg "github.com/conductorone/baton-newrelic/pkg/config"
	"github.com/conductorone/baton-newrelic/pkg/connector"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/connectorrunner"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

var version = "dev"

func main() {
	ctx := context.Background()

	config.RunConnector(
		ctx,
		"baton-newrelic",
		version,
		cfg.Config,
		getConnector,
		connectorrunner.WithDefaultCapabilitiesConnectorBuilderV2(&connector.NewRelic{}),
	)
}

func getConnector(ctx context.Context, cc *cfg.Newrelic, opts *cli.ConnectorOpts) (connectorbuilder.ConnectorBuilderV2, []connectorbuilder.Opt, error) {
	l := ctxzap.Extract(ctx)
	if err := cfg.ValidateConfig(cc); err != nil {
		return nil, nil, err
	}

	cb, err := connector.New(ctx, cc.Apikey, cc.BaseUrl)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, nil, err
	}

	return cb, nil, nil
}
