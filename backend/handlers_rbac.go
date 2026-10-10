package main

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
)

const (
	PermManageOrg            = "manage_org"
	PermManageBilling        = "manage_billing"
	PermInviteMembers        = "invite_members"
	PermManageRoles          = "manage_roles"
	PermViewAnalytics        = "view_analytics"
	PermCreateProject        = "create_project"
	PermEditProject          = "edit_project"
	PermDeleteProject        = "delete_project"
	PermManageProjectMembers = "manage_project_members"
	PermCreateTask           = "create_task"
	PermEditTask             = "edit_task"
	PermDeleteTask           = "delete_task"
	PermAssignTask           = "assign_task"
	PermEditComments         = "edit_comments"
	PermCreateTimeEntries    = "create_time_entries"
	PermViewTimeEntries      = "view_time_entries"
)

var allPermissions = []string{
	PermManageOrg, PermManageBilling, PermInviteMembers, PermManageRoles, PermViewAnalytics,
	PermCreateProject, PermEditProject, PermDeleteProject, PermManageProjectMembers,
	PermCreateTask, PermEditTask, PermDeleteTask, PermAssignTask, PermEditComments,
	PermCreateTimeEntries, PermViewTimeEntries,
}

func getUserPermissions(ctx context.Context, orgID, userID string) ([]string, error) {
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	q := `SELECT DISTINCT p FROM role_assignments ra
		JOIN roles r ON ra.role_id = r.role_id
		CROSS JOIN LATERAL jsonb_array_elements_text(r.permissions) AS p
		WHERE ra.org_id = $1 AND ra.user_id = $2`
	rows, err := tx.Query(ctx, q, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	permSet := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		permSet[p] = true
	}
	perms := make([]string, 0, len(permSet))
	for p := range permSet {
		perms = append(perms, p)
	}
	return perms, nil
}

func hasPermission(ctx context.Context, orgID, userID, perm string) (bool, error) {
	perms, err := getUserPermissions(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	for _, p := range perms {
		if p == perm {
			return true, nil
		}
	}
	return false, nil
}

func listRoles(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if _, herr := ensureMember(c, orgID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	roles, err := pgFindMany(c.Request().Context(), "roles", map[string]any{"org_id": orgID}, nil, "created_at", 100)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, roles)
}

func createRole(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if ok, err := hasPermission(c.Request().Context(), orgID, asStr(user["user_id"]), PermManageRoles); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Name == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Name required")
	}
	validPerms := []string{}
	for _, p := range b.Permissions {
		for _, vp := range allPermissions {
			if p == vp {
				validPerms = append(validPerms, p)
				break
			}
		}
	}
	role := map[string]any{
		"role_id":     newID("role"),
		"org_id":      orgID,
		"name":        b.Name,
		"description": b.Description,
		"permissions": validPerms,
		"is_default":  false,
		"is_system":   false,
		"created_at":  isoNow(),
		"updated_at":  isoNow(),
	}
	if err := pgInsert(c.Request().Context(), "roles", role); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create role")
	}
	return c.JSON(http.StatusCreated, role)
}

func updateRole(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if ok, err := hasPermission(c.Request().Context(), orgID, asStr(user["user_id"]), PermManageRoles); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	roleID := c.Param("role_id")
	ctx := c.Request().Context()
	role, err := pgFindOne(ctx, "roles", map[string]any{"role_id": roleID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if role == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Role not found")
	}
	if asBool(role["is_system"]) {
		return echo.NewHTTPError(http.StatusForbidden, "Cannot edit system role")
	}
	var b struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := bindBody(c, &b); err != nil {
		return err
	}
	set := map[string]any{"updated_at": isoNow()}
	if b.Name != "" {
		set["name"] = b.Name
	}
	if b.Permissions != nil {
		validPerms := []string{}
		for _, p := range b.Permissions {
			for _, vp := range allPermissions {
				if p == vp {
					validPerms = append(validPerms, p)
					break
				}
			}
		}
		set["permissions"] = validPerms
	}
	if b.Description != "" {
		set["description"] = b.Description
	}
	if err := pgUpdate(ctx, "roles", map[string]any{"role_id": roleID}, set); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update role")
	}
	updated, _ := pgFindOne(ctx, "roles", map[string]any{"role_id": roleID}, nil)
	return c.JSON(http.StatusOK, updated)
}

func deleteRole(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if ok, err := hasPermission(c.Request().Context(), orgID, asStr(user["user_id"]), PermManageRoles); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	roleID := c.Param("role_id")
	ctx := c.Request().Context()
	role, err := pgFindOne(ctx, "roles", map[string]any{"role_id": roleID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if role == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Role not found")
	}
	if asBool(role["is_system"]) {
		return echo.NewHTTPError(http.StatusForbidden, "Cannot delete system role")
	}
	if err := pgDelete(ctx, "roles", map[string]any{"role_id": roleID}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete role")
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func assignRole(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if ok, err := hasPermission(c.Request().Context(), orgID, asStr(user["user_id"]), PermManageRoles); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	targetUserID := c.Param("user_id")
	var b struct {
		RoleID string `json:"role_id"`
	}
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.RoleID == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "role_id required")
	}
	ctx := c.Request().Context()
	role, err := pgFindOne(ctx, "roles", map[string]any{"role_id": b.RoleID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if role == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Role not found")
	}
	existing, err := pgFindOne(ctx, "role_assignments", map[string]any{"org_id": orgID, "user_id": targetUserID, "role_id": b.RoleID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if existing != nil {
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	}
	assignment := map[string]any{
		"assignment_id": newID("ra"),
		"org_id":        orgID,
		"user_id":       targetUserID,
		"role_id":       b.RoleID,
		"created_at":    isoNow(),
	}
	if err := pgInsert(ctx, "role_assignments", assignment); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to assign role")
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func removeRole(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if ok, err := hasPermission(c.Request().Context(), orgID, asStr(user["user_id"]), PermManageRoles); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	targetUserID := c.Param("user_id")
	roleID := c.Param("role_id")
	ctx := c.Request().Context()
	if err := pgDelete(ctx, "role_assignments", map[string]any{"org_id": orgID, "user_id": targetUserID, "role_id": roleID}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to remove role")
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func getUserRoles(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if _, herr := ensureMember(c, orgID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	targetUserID := c.Param("user_id")
	ctx := c.Request().Context()
	assignments, err := pgFindMany(ctx, "role_assignments", map[string]any{"org_id": orgID, "user_id": targetUserID}, nil, "created_at", 100)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	roles := []map[string]any{}
	for _, a := range assignments {
		role, err := pgFindOne(ctx, "roles", map[string]any{"role_id": a["role_id"]}, nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if role != nil {
			roles = append(roles, role)
		}
	}
	return c.JSON(http.StatusOK, roles)
}

func listPermissions(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if _, herr := ensureMember(c, orgID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	return c.JSON(http.StatusOK, map[string]any{"permissions": allPermissions})
}
