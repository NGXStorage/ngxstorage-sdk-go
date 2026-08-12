package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// PoolService manages storage pools.
type PoolService struct{ c *Client }

// Pools returns the pool service.
func (c *Client) Pools() *PoolService { return &PoolService{c} }

// List fetches all pools.
func (s *PoolService) List(ctx context.Context) ([]Pool, error) {
	body, err := s.c.get(ctx, "GetPoolList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list pools: %w", err)
	}
	var pools []Pool
	if err := json.Unmarshal(body, &pools); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode pool list: %w", err)
	}
	return pools, nil
}

// Get fetches a single pool.
func (s *PoolService) Get(ctx context.Context, id string) (*Pool, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetPool", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get pool: %w", err)
	}
	var pool Pool
	if err := json.Unmarshal(body, &pool); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode pool: %w", err)
	}
	return &pool, nil
}

// Overview returns the raw pool overview document.
func (s *PoolService) Overview(ctx context.Context) (map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetPoolOverview", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get pool overview: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode pool overview: %w", err)
	}
	return m, nil
}

// GetConfiguredCapacity returns available and reserved bytes for the
// configured pool from the status/capacity endpoint.
func (s *PoolService) GetConfiguredCapacity(ctx context.Context) (avail, reserved int64, err error) {
	body, err := s.c.get(ctx, "GetCapacityStatus", nil)
	if err != nil {
		return 0, 0, fmt.Errorf("ngx sdk: get capacity: %w", err)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, 0, fmt.Errorf("ngx sdk: decode capacity: %w", err)
	}
	for _, p := range raw {
		name, _ := p["name"].(string)
		if name != s.c.poolName {
			continue
		}
		logical, _ := p["logical"].(map[string]interface{})
		if logical == nil {
			break
		}
		avail = bytesFromValue(logical["available"])
		reserved = bytesFromValue(logical["reserved"])
		return avail, reserved, nil
	}
	return 0, 0, fmt.Errorf("ngx sdk: configured pool %q capacity not found", s.c.poolName)
}

// bytesFromValue converts a flexible JSON numeric/string byte value to int64.
func bytesFromValue(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		var i int64
		fmt.Sscanf(n, "%d", &i)
		return i
	default:
		return 0
	}
}
