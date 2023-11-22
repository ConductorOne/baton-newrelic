package connector

import (
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
)

const ResourcesPageSize uint = 50

func annotationsForUserResourceType() annotations.Annotations {
	annos := annotations.Annotations{}
	annos.Update(&v2.SkipEntitlementsAndGrants{})
	return annos
}

func parsePageToken(i string, resourceID *v2.ResourceId) (*pagination.Bag, error) {
	b := &pagination.Bag{}
	err := b.Unmarshal(i)
	if err != nil {
		return nil, err
	}

	if b.Current() == nil {
		b.Push(pagination.PageState{
			ResourceTypeID: resourceID.ResourceType,
			ResourceID:     resourceID.Resource,
		})
	}

	return b, nil
}

func composeCursor(domainId, groupC string) (string, error) {
	if domainId == "" && groupC == "" {
		return "", fmt.Errorf("domainId and groupCursor cannot both be empty")
	}

	if domainId == "" {
		return "", fmt.Errorf("domainId cannot be empty")
	}

	if groupC == "" {
		return "", nil
	}

	return fmt.Sprintf("%s:%s", domainId, groupC), nil
}
