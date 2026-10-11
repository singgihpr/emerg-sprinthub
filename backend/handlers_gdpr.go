package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
)

// auditLog records a security-relevant event for SOC 2 compliance.
func auditLog(ctx context.Context, action, resourceType, resourceID string, details map[string]any) {
	doc := map[string]any{
		"log_id":        newID("log"),
		"action":        action,
		"resource_type": resourceType,
		"resource_id":   resourceID,
		"created_at":    nowUTC(),
	}
	if details != nil {
		b, _ := json.Marshal(details)
		doc["details"] = string(b)
	}
	_ = pgPoolInsert(ctx, "audit_logs", doc)
}

// auditLogRequest extracts request metadata for the audit log.
func auditLogRequest(c echo.Context, action, resourceType, resourceID string, userID, orgID string) {
	doc := map[string]any{
		"log_id":        newID("log"),
		"user_id":       userID,
		"org_id":        orgID,
		"action":        action,
		"resource_type": resourceType,
		"resource_id":   resourceID,
		"ip_address":    c.RealIP(),
		"user_agent":    c.Request().UserAgent(),
		"created_at":    nowUTC(),
	}
	_ = pgPoolInsert(c.Request().Context(), "audit_logs", doc)
}

// exportUserData returns all personal data for the current user (GDPR Article 20).
func exportUserData(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	userID := asStr(user["user_id"])

	memberships, _ := pgPoolFind(ctx, "memberships", map[string]any{"user_id": userID})
	tasksCreated, _ := pgPoolFind(ctx, "tasks", map[string]any{"created_by": userID})
	tasksAssigned, _ := pgPoolFind(ctx, "tasks", map[string]any{"assignee_id": userID})
	comments, _ := pgPoolFind(ctx, "comments", map[string]any{"user_id": userID})
	timeEntries, _ := pgPoolFind(ctx, "time_entries", map[string]any{"user_id": userID})
	projectMembers, _ := pgPoolFind(ctx, "project_members", map[string]any{"user_id": userID})

	export := map[string]any{
		"user":            user,
		"memberships":     memberships,
		"tasks_created":   tasksCreated,
		"tasks_assigned":  tasksAssigned,
		"comments":        comments,
		"time_entries":    timeEntries,
		"project_members": projectMembers,
		"exported_at":     nowUTC().Format("2006-01-02T15:04:05Z"),
	}

	auditLogRequest(c, "user.data_export", "user", userID, userID, "")
	return c.JSON(http.StatusOK, export)
}

// deleteAccount deletes the current user and all related data (GDPR Article 17).
// Cascades via FK constraints handle most cleanup.
func deleteAccount(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	userID := asStr(user["user_id"])

	var b struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.Bind(&b)
	if !b.Confirm {
		return echo.NewHTTPError(http.StatusBadRequest, "Must confirm deletion with {\"confirm\": true}")
	}

	orgs, _ := pgPoolFind(ctx, "organizations", map[string]any{"owner_id": userID})
	if len(orgs) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Transfer ownership of your organizations before deleting your account")
	}

	auditLogRequest(c, "user.delete", "user", userID, userID, "")

	_ = pgPoolExec(ctx, "DELETE FROM users WHERE user_id = $1", userID)

	clearAuthCookie(c)
	clearRefreshCookie(c)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "Account deleted"})
}
