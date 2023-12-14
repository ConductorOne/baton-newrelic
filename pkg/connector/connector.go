package connector

import (
	"context"
	"fmt"
	"io"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
)

type NewRelic struct {
	client *newrelic.Client
}

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (nr *NewRelic) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	return []connectorbuilder.ResourceSyncer{
		newOrgBuilder(nr.client),
		newUserBuilder(nr.client),
		newGroupBuilder(nr.client),
		newRoleBuilder(nr.client),
	}
}

// Asset takes an input AssetRef and attempts to fetch it using the connector's authenticated http client
// It streams a response, always starting with a metadata object, following by chunked payloads for the asset.
func (nr *NewRelic) Asset(ctx context.Context, asset *v2.AssetRef) (string, io.ReadCloser, error) {
	return "", nil, nil
}

// Metadata returns metadata about the connector.
func (nr *NewRelic) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "NewRelic Connector",
		Description: "Connector syncing NewRelic organizations, users, groups and roles to Baton",
	}, nil
}

// Validate is called to ensure that the connector is properly configured. It should exercise any API credentials
// to be sure that they are valid.
func (nr *NewRelic) Validate(ctx context.Context) (annotations.Annotations, error) {
	_, err := nr.client.GetOrg(ctx)
	if err != nil {
		return nil, fmt.Errorf("newrelic-connector: failed to retrieve org: %w", err)
	}

	return nil, nil
}

// New returns a new instance of the connector.
func New(ctx context.Context, apikey string) (*NewRelic, error) {
	httpClient, err := uhttp.NewClient(ctx, uhttp.WithLogger(true, ctxzap.Extract(ctx)))
	if err != nil {
		return nil, err
	}

	nrClient, err := newrelic.NewClient(ctx, httpClient, apikey)
	if err != nil {
		return nil, err
	}

	return &NewRelic{
		client: nrClient,
	}, nil
}
