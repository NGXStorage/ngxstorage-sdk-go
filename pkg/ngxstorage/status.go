package ngxstorage

import (
	"context"
	"encoding/json"
	"fmt"
)

// StatusService queries cluster, services, capacity, and performance state.
type StatusService struct{ c *Client }

// Status returns the status service.
func (c *Client) Status() *StatusService { return &StatusService{c} }

// Cluster returns the cluster work-mode status.
func (s *StatusService) Cluster(ctx context.Context) (*ClusterStatus, error) {
	body, err := s.c.get(ctx, "GetClusterStatus", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get cluster status: %w", err)
	}
	var cluster ClusterStatus
	if err := json.Unmarshal(body, &cluster); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode cluster status: %w", err)
	}
	return &cluster, nil
}

// Services returns the raw services status document.
func (s *StatusService) Services(ctx context.Context) (map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetServicesStatus", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get services status: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode services status: %w", err)
	}
	return m, nil
}

// Capacity returns the raw capacity status document.
func (s *StatusService) Capacity(ctx context.Context) ([]map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetCapacityStatus", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get capacity: %w", err)
	}
	var m []map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode capacity: %w", err)
	}
	return m, nil
}

// IOPS returns the raw IOPS status document.
func (s *StatusService) IOPS(ctx context.Context) (map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetIOPSStatus", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get iops: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode iops: %w", err)
	}
	return m, nil
}

// Bandwidth returns the raw bandwidth status document.
func (s *StatusService) Bandwidth(ctx context.Context) (map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetBandwidth", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get bandwidth: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode bandwidth: %w", err)
	}
	return m, nil
}
