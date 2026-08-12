package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
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

// Get fetches a single FC target with ports and LUN mappings.
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

// GetWWPNs returns the deduplicated target WWPNs with the "naa." prefix
// stripped, suitable for host-side device discovery.
func (s *FCTargetService) GetWWPNs(ctx context.Context, id string) ([]string, error) {
	target, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var wwpns []string
	for _, port := range target.Ports {
		for _, w := range port.Wwpns {
			clean := trimNAA(w.WWPN)
			if clean != "" && !seen[clean] {
				seen[clean] = true
				wwpns = append(wwpns, clean)
			}
		}
	}
	return wwpns, nil
}

// GetLUNNumber returns the SCSI LUN number for a LUN mapped to this target.
func (s *FCTargetService) GetLUNNumber(ctx context.Context, id, lunID string) (int, error) {
	target, err := s.Get(ctx, id)
	if err != nil {
		return -1, err
	}
	for _, lun := range target.Luns {
		if lun.ID == lunID {
			n, err := strconv.Atoi(lun.Number)
			if err != nil {
				return -1, fmt.Errorf("ngx sdk: parse LUN number %q: %w", lun.Number, err)
			}
			return n, nil
		}
	}
	return -1, fmt.Errorf("ngx sdk: LUN %s not mapped to target %s", lunID, id)
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

// SelectByName returns the ID of the first pre-created FC target matching name.
func (s *FCTargetService) SelectByName(ctx context.Context, name string) (string, error) {
	targets, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range targets {
		if t.Name == name {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("ngx sdk: FC target %q not found", name)
}
