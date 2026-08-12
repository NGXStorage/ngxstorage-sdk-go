package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// FCTargetService manages Fibre Channel targets and LUN mappings.
type FCTargetService struct{ c *Client }

// FCTargets returns the FC target service.
func (c *Client) FCTargets() *FCTargetService { return &FCTargetService{c} }

// List fetches all FC targets (lightweight list).
func (s *FCTargetService) List(ctx context.Context) ([]FCTarget, error) {
	body, err := s.c.get(ctx, "GetFCTargetList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list FC targets: %w", err)
	}
	var targets []FCTarget
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode FC target list: %w", err)
	}
	return targets, nil
}

// Get fetches a single FC target with ports and LUN mappings. Field values
// are returned exactly as the backend produces them (WWPNs retain the "naa."
// prefix; LUN numbers are strings). Interpretation is the driver's job.
func (s *FCTargetService) Get(ctx context.Context, id string) (*FCTarget, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetFCTarget", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get FC target: %w", err)
	}
	var target FCTarget
	if err := json.Unmarshal(body, &target); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode FC target: %w", err)
	}
	return &target, nil
}

// AddLUN maps a LUN to a pre-created FC target. Idempotent.
func (s *FCTargetService) AddLUN(ctx context.Context, targetID, lunID, owner string) error {
	vars := url.Values{"id": {targetID}}
	body := map[string]interface{}{"owner": owner, "lun_ids": []string{lunID}}
	_, err := s.c.mutation(ctx, "AddLunToFCTarget", vars, body)
	if IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: add LUN to FC target: %w", err)
	}
	return nil
}

// RemoveLUN unmaps a LUN from an FC target. Idempotent.
func (s *FCTargetService) RemoveLUN(ctx context.Context, targetID, lunID, owner string) error {
	vars := url.Values{"id": {targetID}}
	body := map[string]interface{}{"owner": owner, "lun_ids": []string{lunID}}
	_, err := s.c.mutation(ctx, "RemoveLunFromFCTarget", vars, body)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: remove LUN from FC target: %w", err)
	}
	return nil
}
