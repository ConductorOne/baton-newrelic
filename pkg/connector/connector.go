package connector

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	configv1 "github.com/conductorone/baton-sdk/pb/c1/config/v1"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/actions"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	actionArgUserID = "user_id"
)

type NewRelic struct {
	client                 *newrelic.Client
	authenticationDomainID string
}

// ResourceSyncers returns a ResourceSyncerV2 for each resource type that should be synced from the upstream service.
func (nr *NewRelic) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncerV2 {
	return []connectorbuilder.ResourceSyncerV2{
		newOrgBuilder(nr.client),
		newUserBuilder(nr.client, nr.authenticationDomainID),
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
	authDomainField := v2.ConnectorAccountCreationSchema_StringField_builder{}.Build()
	if nr.authenticationDomainID != "" {
		authDomainField.SetDefaultValue(nr.authenticationDomainID)
	}

	schemaFields := map[string]*v2.ConnectorAccountCreationSchema_Field{
		"name": v2.ConnectorAccountCreationSchema_Field_builder{
			DisplayName: "Full Name",
			Description: "The user's full display name",
			Required:    true,
			Order:       1,
			StringField: &v2.ConnectorAccountCreationSchema_StringField{},
		}.Build(),
		"email": v2.ConnectorAccountCreationSchema_Field_builder{
			DisplayName: "Email Address",
			Description: "The user's email address (used as login)",
			Required:    true,
			Order:       2,
			StringField: &v2.ConnectorAccountCreationSchema_StringField{},
		}.Build(),
		"user_type": v2.ConnectorAccountCreationSchema_Field_builder{
			DisplayName: "User Type",
			Description: "The New Relic user tier: BASIC_USER_TIER, CORE_USER_TIER, or FULL_USER_TIER",
			Required:    true,
			Order:       3,
			StringField: &v2.ConnectorAccountCreationSchema_StringField{},
		}.Build(),
		"authentication_domain_id": v2.ConnectorAccountCreationSchema_Field_builder{
			DisplayName: "Authentication Domain ID",
			Description: "The ID of the New Relic authentication domain in which to create the user",
			Required:    true,
			Order:       4,
			StringField: authDomainField,
		}.Build(),
	}

	return &v2.ConnectorMetadata{
		DisplayName:           "NewRelic Connector",
		Description:           "Connector syncing NewRelic organizations, users, groups and roles to Baton",
		AccountCreationSchema: &v2.ConnectorAccountCreationSchema{FieldMap: schemaFields},
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

// GlobalActions registers the update_user action for changing user profile attributes.
func (nr *NewRelic) GlobalActions(ctx context.Context, registry actions.ActionRegistry) error {
	schema := v2.BatonActionSchema_builder{
		Name:        "update_user",
		DisplayName: "Update User",
		Description: "Update a New Relic user's profile attributes (name, email, or userType)",
		ActionType:  []v2.ActionType{v2.ActionType_ACTION_TYPE_ACCOUNT_UPDATE_PROFILE},
		Arguments: []*configv1.Field{
			configv1.Field_builder{
				Name:        actionArgUserID,
				DisplayName: "User ID",
				Description: "The New Relic user ID to update",
				IsRequired:  true,
				ResourceIdField: configv1.ResourceIdField_builder{
					Rules: configv1.ResourceIDRules_builder{
						AllowedResourceTypeIds: []string{"user"},
					}.Build(),
				}.Build(),
			}.Build(),
			configv1.Field_builder{
				Name:        "name",
				DisplayName: "Full Name",
				Description: "New display name for the user",
				IsRequired:  false,
				StringField: &configv1.StringField{},
			}.Build(),
			configv1.Field_builder{
				Name:        "email",
				DisplayName: "Email Address",
				Description: "New email address for the user",
				IsRequired:  false,
				StringField: &configv1.StringField{},
			}.Build(),
			configv1.Field_builder{
				Name:        "user_type",
				DisplayName: "User Type",
				Description: "New user tier (BASIC_USER_TIER, CORE_USER_TIER, or FULL_USER_TIER)",
				IsRequired:  false,
				StringField: &configv1.StringField{},
			}.Build(),
		},
		ReturnTypes: []*configv1.Field{
			configv1.Field_builder{
				Name:        "success",
				DisplayName: "Success",
				BoolField:   &configv1.BoolField{},
			}.Build(),
			configv1.Field_builder{
				Name:        actionArgUserID,
				DisplayName: "User ID",
				StringField: &configv1.StringField{},
			}.Build(),
		},
	}.Build()

	return registry.Register(ctx, schema, func(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
		l := ctxzap.Extract(ctx)

		userResourceID, ok := actions.GetResourceIDArg(args, actionArgUserID)
		if !ok || userResourceID == nil {
			return nil, nil, fmt.Errorf("baton-newrelic: update_user action requires user_id")
		}
		userID := userResourceID.Resource

		name, _ := actions.GetStringArg(args, "name")
		email, _ := actions.GetStringArg(args, "email")
		userType, _ := actions.GetStringArg(args, "user_type")

		if name == "" && email == "" && userType == "" {
			return nil, nil, fmt.Errorf("baton-newrelic: update_user requires at least one of name, email, or user_type")
		}

		l.Debug("update_user action invoked",
			zap.String(actionArgUserID, userID),
			zap.String("name", name),
			zap.String("email", email),
			zap.String("user_type", userType),
		)

		if err := nr.client.UpdateUser(ctx, userID, email, name, userType); err != nil {
			return nil, nil, fmt.Errorf("baton-newrelic: update_user failed: %w", err)
		}

		rv := actions.NewReturnValues(true, actions.NewStringReturnField(actionArgUserID, userID))
		return rv, nil, nil
	})
}

// New returns a new instance of the connector.
func New(ctx context.Context, apikey string, baseURL string, authDomainID string) (*NewRelic, error) {
	var httpClient *http.Client
	var err error

	if apikey != "" {
		httpClient, err = uhttp.NewClient(ctx, uhttp.WithLogger(true, ctxzap.Extract(ctx)))
		if err != nil {
			return nil, err
		}
	}

	nrClient, err := newrelic.NewClient(ctx, httpClient, apikey, baseURL)
	if err != nil {
		return nil, err
	}

	return &NewRelic{
		client:                 nrClient,
		authenticationDomainID: authDomainID,
	}, nil
}
