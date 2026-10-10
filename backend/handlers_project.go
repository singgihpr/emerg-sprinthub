package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// accessibleProjectIDs mirrors python _get_accessible_project_ids:
// all=true means the user may see every project (owner/admin).
func accessibleProjectIDs(c echo.Context, orgID, userID, role string) ([]string, bool, *echo.HTTPError) {
	if role == "owner" || role == "admin" {
		return nil, true, nil
	}
	pm, err := pgFindMany(c.Request().Context(), "project_members",
		map[string]any{"org_id": orgID, "user_id": userID}, nil, "", 500)
	if err != nil {
		return nil, false, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	ids := make([]string, 0, len(pm))
	for _, p := range pm {
		ids = append(ids, asStr(p["project_id"]))
	}
	return ids, false, nil
}

func requireProjectAccess(c echo.Context, orgID, projectID, userID string) (map[string]any, *echo.HTTPError) {
	m, herr := ensureMember(c, orgID, userID)
	if herr != nil {
		return nil, herr
	}
	if !roleIs(m, "owner", "admin") {
		pm, err := pgFindOne(c.Request().Context(), "project_members",
			map[string]any{"org_id": orgID, "project_id": projectID, "user_id": userID}, nil)
		if err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if pm == nil {
			return nil, echo.NewHTTPError(http.StatusForbidden, "No access to this project")
		}
	}
	return m, nil
}

func requireTaskAccess(c echo.Context, orgID, taskID, userID string) (map[string]any, *echo.HTTPError) {
	task, err := pgFindOne(c.Request().Context(), "tasks",
		map[string]any{"task_id": taskID, "org_id": orgID}, nil)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if task == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}
	if _, herr := requireProjectAccess(c, orgID, asStr(task["project_id"]), userID); herr != nil {
		return nil, herr
	}
	return task, nil
}

// ---- projects ----

func listProjects(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	allowed, all, herr := accessibleProjectIDs(c, orgID, asStr(user["user_id"]), asStr(m["role"]))
	if herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	var projects []map[string]any
	var err error
	if all {
		projects, err = pgFindMany(ctx, "projects", map[string]any{"org_id": orgID}, nil, "", 500)
	} else {
		if len(allowed) == 0 {
			return c.JSON(http.StatusOK, []map[string]any{})
		}
		placeholders := make([]string, len(allowed))
		args := []any{orgID}
		for i, id := range allowed {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, id)
		}
		q := fmt.Sprintf("SELECT * FROM projects WHERE org_id = $1 AND project_id IN (%s) LIMIT 500",
			strings.Join(placeholders, ","))
		rows, e := pgExecQuery(ctx, q, args...)
		if e != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		projects = rows
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, projects)
}

func createProject(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermCreateProject); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b projectCreate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Name == "" || b.Key == "" || (b.Status != nil && !vStatus(*b.Status, projectStatuses)) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	if herr := checkQuota(ctx, orgID, "projects", 1); herr != nil {
		return herr
	}
	status := "active"
	if b.Status != nil {
		status = *b.Status
	}
	projectID := newID("prj")
	doc := map[string]any{
		"project_id": projectID, "org_id": orgID,
		"name": b.Name, "key": strings.ToUpper(b.Key),
		"description": b.Description, "color": b.Color,
		"status":     status,
		"created_by": user["user_id"], "created_at": isoNow(),
	}
	if err := pgInsert(ctx, "projects", doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	incrementQuota(ctx, orgID, "projects", 1)
	logUsageEvent(ctx, orgID, "project_created", 1)
	_ = pgInsert(ctx, "project_members", map[string]any{
		"project_member_id": newID("pmem"), "org_id": orgID, "project_id": projectID, "user_id": user["user_id"],
		"role": "lead", "added_at": isoNow(), "added_by": user["user_id"],
	})
	return c.JSON(http.StatusOK, doc)
}

func updateProject(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	projectID := c.Param("project_id")
	userID := asStr(user["user_id"])
	if _, herr := requireProjectAccess(c, orgID, projectID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermEditProject); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b projectUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Status != nil && !vStatus(*b.Status, projectStatuses) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	updates := map[string]any{}
	if b.Name != nil {
		updates["name"] = *b.Name
	}
	if b.Description != nil {
		updates["description"] = *b.Description
	}
	if b.Color != nil {
		updates["color"] = *b.Color
	}
	if b.Status != nil {
		updates["status"] = *b.Status
	}
	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "No fields to update")
	}
	ctx := c.Request().Context()
	if err := pgUpdate(ctx, "projects", map[string]any{"project_id": projectID, "org_id": orgID}, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	updated, err := pgFindOne(ctx, "projects", map[string]any{"project_id": projectID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if updated == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Project not found")
	}
	return c.JSON(http.StatusOK, updated)
}

func deleteProject(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	projectID := c.Param("project_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermDeleteProject); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	ctx := c.Request().Context()
	project, err := pgFindOne(ctx, "projects", map[string]any{"project_id": projectID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Project not found")
	}
	// Cascade: collect task ids, then delete children
	tasks, err := pgFindMany(ctx, "tasks", map[string]any{"org_id": orgID, "project_id": projectID}, []string{"task_id"}, "", 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, asStr(t["task_id"]))
	}
	_ = pgDelete(ctx, "tasks", map[string]any{"org_id": orgID, "project_id": projectID})
	_ = pgDelete(ctx, "sprints", map[string]any{"org_id": orgID, "project_id": projectID})
	_ = pgDelete(ctx, "project_members", map[string]any{"org_id": orgID, "project_id": projectID})
	if len(taskIDs) > 0 {
		placeholders := make([]string, len(taskIDs))
		args := []any{orgID}
		for i, id := range taskIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, id)
		}
		q := fmt.Sprintf("DELETE FROM time_entries WHERE org_id = $1 AND task_id IN (%s)", strings.Join(placeholders, ","))
		_, _ = pgExecRaw(ctx, q, args...)
		q = fmt.Sprintf("DELETE FROM comments WHERE org_id = $1 AND task_id IN (%s)", strings.Join(placeholders, ","))
		_, _ = pgExecRaw(ctx, q, args...)
		q = fmt.Sprintf("DELETE FROM active_timers WHERE org_id = $1 AND task_id IN (%s)", strings.Join(placeholders, ","))
		_, _ = pgExecRaw(ctx, q, args...)
	}
	_ = pgDelete(ctx, "projects", map[string]any{"project_id": projectID, "org_id": orgID})
	decrementQuota(ctx, orgID, "projects", 1)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "deleted_tasks": len(taskIDs)})
}

// ---- project members ----

func listProjectMembers(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	projectID := c.Param("project_id")
	if _, herr := requireProjectAccess(c, orgID, projectID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	members, err := pgFindMany(ctx, "project_members", map[string]any{"org_id": orgID, "project_id": projectID}, nil, "", 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, asStr(m["user_id"]))
	}
	umap := map[string]map[string]any{}
	for _, uid := range userIDs {
		u, err := pgFindOne(ctx, "users", map[string]any{"user_id": uid}, []string{"user_id", "email", "name", "picture", "created_at"})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if u != nil {
			umap[uid] = u
		}
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		u := umap[asStr(m["user_id"])]
		if u == nil {
			u = map[string]any{}
		}
		row := map[string]any{}
		for k, v := range u {
			row[k] = v
		}
		role := mapStr(m, "role", "member")
		row["project_role"] = role
		row["added_at"] = m["added_at"]
		out = append(out, row)
	}
	return c.JSON(http.StatusOK, out)
}

func addProjectMember(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	projectID := c.Param("project_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermManageProjectMembers); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b projectMemberAdd
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.UserID == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	target, err := pgFindOne(ctx, "memberships", map[string]any{"org_id": orgID, "user_id": b.UserID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if target == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "User is not a member of the organization")
	}
	existing, err := pgFindOne(ctx, "project_members", map[string]any{"org_id": orgID, "project_id": projectID, "user_id": b.UserID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if existing != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Already a project member")
	}
	role := "member"
	if b.Role != nil && *b.Role != "" {
		role = *b.Role
	}
	if err := pgInsert(ctx, "project_members", map[string]any{
		"project_member_id": newID("pmem"), "org_id": orgID, "project_id": projectID, "user_id": b.UserID,
		"role": role, "added_at": isoNow(), "added_by": user["user_id"],
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func removeProjectMember(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	projectID := c.Param("project_id")
	targetUserID := c.Param("user_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermManageProjectMembers); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	_ = pgDelete(c.Request().Context(), "project_members",
		map[string]any{"org_id": orgID, "project_id": projectID, "user_id": targetUserID})
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// ---- sprints ----

func listSprints(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	allowed, all, herr := accessibleProjectIDs(c, orgID, asStr(user["user_id"]), asStr(m["role"]))
	if herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	filter := map[string]any{"org_id": orgID}
	projectID := c.QueryParam("project_id")
	if projectID != "" {
		// further narrow, still respecting ACL
		if !all {
			found := false
			for _, a := range allowed {
				if a == projectID {
					found = true
					break
				}
			}
			if !found {
				return c.JSON(http.StatusOK, []map[string]any{})
			}
		}
		filter["project_id"] = projectID
	}
	status := c.QueryParam("status")
	if status != "" {
		filter["status"] = status
	}
	var sprints []map[string]any
	var err error
	if all || projectID != "" {
		sprints, err = pgFindMany(ctx, "sprints", filter, nil, "created_at DESC", 500)
	} else {
		if len(allowed) == 0 {
			return c.JSON(http.StatusOK, []map[string]any{})
		}
		placeholders := make([]string, len(allowed))
		args := []any{orgID}
		for i, id := range allowed {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, id)
		}
		q := fmt.Sprintf("SELECT * FROM sprints WHERE org_id = $1 AND project_id IN (%s) ORDER BY created_at DESC LIMIT 500",
			strings.Join(placeholders, ","))
		rows, e := pgExecQuery(ctx, q, args...)
		if e != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		sprints = rows
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, sprints)
}

func createSprint(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	var b sprintCreate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.ProjectID == "" || b.Name == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	userID := asStr(user["user_id"])
	if _, herr := requireProjectAccess(c, orgID, b.ProjectID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermCreateTask); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	doc := map[string]any{
		"sprint_id": newID("spr"), "org_id": orgID, "project_id": b.ProjectID,
		"name": b.Name, "goal": b.Goal,
		"start_date": b.StartDate, "end_date": b.EndDate,
		"status": "planned", "created_at": isoNow(),
	}
	if err := pgInsert(c.Request().Context(), "sprints", doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, doc)
}

func updateSprint(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	sprintID := c.Param("sprint_id")
	ctx := c.Request().Context()
	sprint, err := pgFindOne(ctx, "sprints", map[string]any{"sprint_id": sprintID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if sprint == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Sprint not found")
	}
	userID := asStr(user["user_id"])
	if _, herr := requireProjectAccess(c, orgID, asStr(sprint["project_id"]), userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermEditTask); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b sprintUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Status != nil && !vStatus(*b.Status, sprintStatuses) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	updates := map[string]any{}
	if b.Name != nil {
		updates["name"] = *b.Name
	}
	if b.Goal != nil {
		updates["goal"] = *b.Goal
	}
	if b.StartDate != nil {
		updates["start_date"] = *b.StartDate
	}
	if b.EndDate != nil {
		updates["end_date"] = *b.EndDate
	}
	if b.Status != nil {
		updates["status"] = *b.Status
	}
	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "No fields to update")
	}
	if err := pgUpdate(ctx, "sprints", map[string]any{"sprint_id": sprintID, "org_id": orgID}, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	updated, err := pgFindOne(ctx, "sprints", map[string]any{"sprint_id": sprintID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, updated)
}

func deleteSprint(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	sprintID := c.Param("sprint_id")
	ctx := c.Request().Context()
	sprint, err := pgFindOne(ctx, "sprints", map[string]any{"sprint_id": sprintID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if sprint == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Sprint not found")
	}
	userID := asStr(user["user_id"])
	if _, herr := requireProjectAccess(c, orgID, asStr(sprint["project_id"]), userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermDeleteTask); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	_ = pgUpdate(ctx, "tasks",
		map[string]any{"org_id": orgID, "sprint_id": sprintID},
		map[string]any{"sprint_id": nil, "former_sprint_name": sprint["name"]})
	_ = pgDelete(ctx, "sprints", map[string]any{"sprint_id": sprintID, "org_id": orgID})
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
