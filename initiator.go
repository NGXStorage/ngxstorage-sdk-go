package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// InitiatorService lists FC and iSCSI initiators.
type InitiatorService struct{ c *Client }

// Initiators returns the initiator service.
func (c *Client) Initiators() *InitiatorService { return &InitiatorService{c} }

// List fetches all initiators.
func (s *InitiatorService) List(ctx context.Context) ([]Initiator, error) {
	body, err := s.c.get(ctx, "GetInitiators", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list initiators: %w", err)
	}
	var inits []Initiator
	if err := json.Unmarshal(body, &inits); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode initiators: %w", err)
	}
	return inits, nil
}

// GetFC fetches FC initiators for a target.
func (s *InitiatorService) GetFC(ctx context.Context, id string) ([]Initiator, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetFCInitiators", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get FC initiators: %w", err)
	}
	var inits []Initiator
	if err := json.Unmarshal(body, &inits); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode FC initiators: %w", err)
	}
	return inits, nil
}

// GetISCSI fetches iSCSI initiators for a target.
func (s *InitiatorService) GetISCSI(ctx context.Context, id string) ([]Initiator, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetISCSIInitiators", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get iSCSI initiators: %w", err)
	}
	var inits []Initiator
	if err := json.Unmarshal(body, &inits); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode iSCSI initiators: %w", err)
	}
	return inits, nil
}
