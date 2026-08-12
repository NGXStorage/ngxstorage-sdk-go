package ngxstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// PortalGroupService manages iSCSI portal groups.
type PortalGroupService struct{ c *Client }

// PortalGroups returns the portal group service.
func (c *Client) PortalGroups() *PortalGroupService { return &PortalGroupService{c} }

// List fetches all portal groups.
func (s *PortalGroupService) List(ctx context.Context) ([]PortalGroup, error) {
	body, err := s.c.get(ctx, "GetPortalGroupList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: list portal groups: %w", err)
	}
	var groups []PortalGroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode portal group list: %w", err)
	}
	return groups, nil
}

// Get fetches a single portal group.
func (s *PortalGroupService) Get(ctx context.Context, id string) (*PortalGroup, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetPortalGroup", vars)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get portal group: %w", err)
	}
	var group PortalGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode portal group: %w", err)
	}
	return &group, nil
}

// Create provisions a portal group.
func (s *PortalGroupService) Create(ctx context.Context, name, owner string) error {
	body := map[string]interface{}{"name": name, "owner": owner}
	_, err := s.c.mutation(ctx, "CreatePortalGroup", nil, body)
	if err != nil {
		return fmt.Errorf("ngxstorage: create portal group: %w", err)
	}
	return nil
}

// Delete removes a portal group. Idempotent.
func (s *PortalGroupService) Delete(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeletePortalGroup", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngxstorage: delete portal group: %w", err)
	}
	return nil
}
