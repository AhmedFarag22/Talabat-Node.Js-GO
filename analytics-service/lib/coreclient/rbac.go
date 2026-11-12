package coreclient

import (
	"context"
	"fmt"
	"net/url"
)

// GetPermissionsForRole fetches the permissions list for a role from
// core-service. Used as the loader for lib/rbac.Cache.
func (c *Client) GetPermissionsForRole(ctx context.Context, role string) ([]string, error) {
	u := fmt.Sprintf("%s/api/internal/rbac/permissions?role=%s", c.baseURL, url.QueryEscape(role))
	var resp PermissionsResponse
	if err := c.http.GetJSON(ctx, u, c.headers(), &resp); err != nil {
		return nil, fmt.Errorf("core rbac fetch: %w", err)
	}
	return resp.Data.Permissions, nil
}
