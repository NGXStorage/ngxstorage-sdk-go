package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// AuthGroupService manages iSCSI auth groups (CHAP credential containers).
type AuthGroupService struct{ c *Client }

// AuthGroups returns the auth group service.
func (c *Client) AuthGroups() *AuthGroupService { return &AuthGroupService{c} }

// Create provisions an auth group. Returns the new group ID.
func (s *AuthGroupService) Create(ctx context.Context, name, owner string) (string, error) {
	body := map[string]interface{}{"name": name, "owner": owner}
	resp, err := s.c.mutation(ctx, "CreateAuthGroup", nil, body)
	if err != nil {
		return "", fmt.Errorf("ngx sdk: create auth group: %w", err)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", fmt.Errorf("ngx sdk: decode create auth group: %w", err)
	}
	return parsed.ID, nil
}

// Get fetches an auth group including its CHAP and initiators.
func (s *AuthGroupService) Get(ctx context.Context, id string) (*AuthGroup, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetAuthGroup", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get auth group: %w", err)
	}
	var group AuthGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode auth group: %w", err)
	}
	return &group, nil
}

// List fetches all auth groups.
func (s *AuthGroupService) List(ctx context.Context) ([]AuthGroup, error) {
	body, err := s.c.get(ctx, "GetAuthGroupList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list auth groups: %w", err)
	}
	var groups []AuthGroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode auth group list: %w", err)
	}
	return groups, nil
}

// Delete removes an auth group. Idempotent.
func (s *AuthGroupService) Delete(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeleteAuthGroup", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: delete auth group: %w", err)
	}
	return nil
}

// AddCHAP attaches CHAP credentials to an auth group. The password is never
// logged by the SDK.
func (s *AuthGroupService) AddCHAP(ctx context.Context, id, username, password string) error {
	vars := url.Values{"id": {id}}
	body := map[string]interface{}{"username": username, "password": password}
	_, err := s.c.mutation(ctx, "AddCHAP", vars, body)
	if err != nil {
		return fmt.Errorf("ngx sdk: add CHAP: %w", err)
	}
	return nil
}

// DeleteCHAP removes CHAP credentials from an auth group.
func (s *AuthGroupService) DeleteCHAP(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeleteCHAP", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: delete CHAP: %w", err)
	}
	return nil
}
