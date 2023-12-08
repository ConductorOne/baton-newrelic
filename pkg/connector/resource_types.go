package connector

import (
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

var (
	// The user resource type is for all user objects across all domains.
	userResourceType = &v2.ResourceType{
		Id:          "user",
		DisplayName: "User",
		Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_USER},
		Annotations: annotationsForUserResourceType(),
	}
	// The org resource type is for organization as top level resource.
	orgResourceType = &v2.ResourceType{
		Id:          "org",
		DisplayName: "Org",
		Annotations: annotationsForUserResourceType(),
	}
	// The role resource type is for all role objects across organization.
	roleResourceType = &v2.ResourceType{
		Id:          "role",
		DisplayName: "Role",
		Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_ROLE},
	}
	// The group resource type is for all group under some specific domain.
	groupResourceType = &v2.ResourceType{
		Id:          "group",
		DisplayName: "Group",
		Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_GROUP},
	}
	// The domain resource type is for all authentication domain objects across organization.
	domainResourceType = "domain"
)
