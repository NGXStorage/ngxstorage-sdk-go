package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ISCSITargetService manages iSCSI targets and LUN mappings.
type ISCSITargetService struct{ c *Client }

// ISCSITargets returns the iSCSI target service.
func (c *Client) ISCSITargets() *ISCSITargetService { return &ISCSITargetService{c} }

// List fetches all iSCSI targets.
func (s *ISCSITargetService) List(ctx context.Context) ([]ISCSITarget, error) {
	body, err := s.c.get(ctx, "GetISCSITargetList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list iSCSI targets: %w", err)
	}
	var targets []ISCSITarget
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode iSCSI target list: %w", err)
	}
	return targets, nil
}

// Get fetches a single iSCSI target.
func (s *ISCSITargetService) Get(ctx context.Context, id string) (*ISCSITarget, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetISCSITarget", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get iSCSI target: %w", err)
	}
	var target ISCSITarget
	if err := json.Unmarshal(body, &target); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode iSCSI target: %w", err)
	}
	return &target, nil
}

// Create provisions a new iSCSI target.
func (s *ISCSITargetService) Create(ctx context.Context, name, owner string) (*ISCSITarget, error) {
	body := map[string]interface{}{"name": name, "owner": owner}
	resp, err := s.c.mutation(ctx, "CreateISCSITarget", nil, body)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: create iSCSI target: %w", err)
	}
	var target ISCSITarget
	if err := json.Unmarshal(resp, &target); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode create iSCSI target: %w", err)
	}
	return &target, nil
}

// Delete removes an iSCSI target. Idempotent.
func (s *ISCSITargetService) Delete(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeleteISCSITarget", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: delete iSCSI target: %w", err)
	}
	return nil
}

// AddLUN maps a LUN to an iSCSI target. Idempotent.
func (s *ISCSITargetService) AddLUN(ctx context.Context, targetID, lunID, owner string) error {
	vars := url.Values{"id": {targetID}}
	body := map[string]interface{}{"owner": owner, "lun_ids": []string{lunID}}
	_, err := s.c.mutation(ctx, "AddLunToISCSITarget", vars, body)
	if IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: add LUN to iSCSI target: %w", err)
	}
	return nil
}

// RemoveLUN unmaps a LUN from an iSCSI target. Idempotent.
func (s *ISCSITargetService) RemoveLUN(ctx context.Context, targetID, lunID, owner string) error {
	vars := url.Values{"id": {targetID}}
	body := map[string]interface{}{"owner": owner, "lun_ids": []string{lunID}}
	_, err := s.c.mutation(ctx, "RemoveLunFromISCSITarget", vars, body)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: remove LUN from iSCSI target: %w", err)
	}
	return nil
}
