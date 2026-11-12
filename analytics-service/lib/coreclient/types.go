package coreclient

// PermissionsResponse is the shape returned by core's
// GET /internal/rbac/permissions?role=<role>. Conventional envelope:
//
//	{ "success": true, "data": { "role": "system_admin", "permissions": [ ... ] } }
type PermissionsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
	} `json:"data"`
}
