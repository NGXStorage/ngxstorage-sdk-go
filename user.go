package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// UserService lists users. User credentials are never present in list
// responses and are never logged.
type UserService struct{ c *Client }

// Users returns the user service.
func (c *Client) Users() *UserService { return &UserService{c} }

// List fetches all users (usernames/roles only, no passwords).
func (s *UserService) List(ctx context.Context) ([]map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetUserList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list users: %w", err)
	}
	var m []map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode user list: %w", err)
	}
	return m, nil
}

// Get fetches a single user (username/roles only, no password).
func (s *UserService) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetUser", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get user: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode user: %w", err)
	}
	return m, nil
}
