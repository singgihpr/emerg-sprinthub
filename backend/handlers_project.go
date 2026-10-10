package main

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
)

// accessibleProjectIDs mirrors python _get_accessible_project_ids:
// all=true means the user may see every project (owner/admin).
func accessibleProjectIDs(c echo.Context, orgID, userID, role string) ([]string, bool, *echo.HTTPError) {
	if role == "owner" || role == "admin" {
		return nil, true, nil
	}
	pm, err := findMany(c.Request().Context(), colProjMem,
		bson.M{"org_id": orgID, "user_id": userID}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return nil, false, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	ids := make([]string, 0, len(pm))
	for _, p := range pm {
		ids = append(ids, asStr(p["project_id"]))
	}
	return ids, false, nil
}

func requireProjectAccess(c echo.Context, orgID, projectID, userID string) (bson.M, *echo.HTTPError) {
	m, herr := ensureMember(c, orgID, userID)
	if herr != nil {
		return nil, herr
	}
	if !roleIs(m, "owner", "admin") {
		pm, err := findOne(c.Request().Context(), colProjMem,
			bson.M{"org_id": orgID, "project_id": projectID, "user_id": userID}, nil)
		if err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if pm == nil {
			return nil, echo.NewHTTPError(http.StatusForbidden, "No access to this project")
		}
	}
	return m, nil
}

func requireTaskAccess(c echo.Context, orgID, taskID, userID string) (bson.M, *echo.HTTPError) {
	task, err := findOne(c.Request().Context(), colTasks,
		bson.M{"task_id": taskID, "org_id": orgID}, bson.M{"_id": 0})
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
	q := bson.M{"org_id": orgID}
	if !all {
		q["project_id"] = bson.M{"$in": allowed}
	}
	projects, err := findMany(c.Request().Context(), colProjects, q, bson.M{"_id": 0}, nil, 500)
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
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin", "manager") {
		return echo.NewHTTPError(http.StatusForbidden, "Requires manager or higher role")
	}
	var b projectCreate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Name == "" || b.Key == "" || (b.Status != nil && !vStatus(*b.Status, projectStatuses)) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	status := "active"
	if b.Status != nil {
		status = *b.Status
	}
	projectID := newID("prj")
	doc := bson.M{
		"project_id": projectID, "org_id": orgID,
		"name": b.Name, "key": strings.ToUpper(b.Key),
		"description": b.Description, "color": b.Color,
		"status":     status,
		"created_by": user["user_id"], "created_at": isoNow(),
	}
	if err := insertDoc(ctx, colProjects, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	_ = insertDoc(ctx, colProjMem, bson.M{
		"org_id": orgID, "project_id": projectID, "user_id": user["user_id"],
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
	m, herr := requireProjectAccess(c, orgID, projectID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin", "manager") {
		return echo.NewHTTPError(http.StatusForbidden, "Requires manager or higher role")
	}
	var b projectUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Status != nil && !vStatus(*b.Status, projectStatuses) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	updates := bson.M{}
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
	res, err := colProjects.UpdateOne(ctx, bson.M{"project_id": projectID, "org_id": orgID}, bson.M{"$set": updates})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if res.MatchedCount == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "Project not found")
	}
	updated, err := findOne(ctx, colProjects, bson.M{"project_id": projectID}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
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
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin") {
		return echo.NewHTTPError(http.StatusForbidden, "Only owner or admin can delete projects")
	}
	ctx := c.Request().Context()
	project, err := findOne(ctx, colProjects, bson.M{"project_id": projectID, "org_id": orgID}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Project not found")
	}
	// Cascade: collect task ids, then delete children
	tasks, err := findMany(ctx, colTasks, bson.M{"org_id": orgID, "project_id": projectID},
		bson.M{"_id": 0, "task_id": 1}, nil, 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, asStr(t["task_id"]))
	}
	_, _ = colTasks.DeleteMany(ctx, bson.M{"org_id": orgID, "project_id": projectID})
	_, _ = colSprints.DeleteMany(ctx, bson.M{"org_id": orgID, "project_id": projectID})
	_, _ = colProjMem.DeleteMany(ctx, bson.M{"org_id": orgID, "project_id": projectID})
	if len(taskIDs) > 0 {
		in := bson.M{"$in": taskIDs}
		_, _ = colEntries.DeleteMany(ctx, bson.M{"org_id": orgID, "task_id": in})
		_, _ = colComments.DeleteMany(ctx, bson.M{"org_id": orgID, "task_id": in})
		_, _ = colTimers.DeleteMany(ctx, bson.M{"org_id": orgID, "task_id": in})
	}
	_, _ = colProjects.DeleteOne(ctx, bson.M{"project_id": projectID, "org_id": orgID})
	return c.JSON(http.StatusOK, bson.M{"ok": true, "deleted_tasks": len(taskIDs)})
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
	members, err := findMany(ctx, colProjMem, bson.M{"org_id": orgID, "project_id": projectID}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, asStr(m["user_id"]))
	}
	umap := map[string]bson.M{}
	for _, uid := range userIDs {
		u, err := pgFindOne(ctx, "users", map[string]any{"user_id": uid}, []string{"user_id", "email", "name", "picture", "created_at"})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if u != nil {
			bm := bson.M{}
			for k, v := range u {
				bm[k] = v
			}
			umap[uid] = bm
		}
	}
	out := make([]bson.M, 0, len(members))
	for _, m := range members {
		u := umap[asStr(m["user_id"])]
		if u == nil {
			u = bson.M{}
		}
		row := bson.M{}
		for k, v := range u {
			row[k] = v
		}
		role := bsonStr(m, "role", "member")
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
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin", "manager") {
		return echo.NewHTTPError(http.StatusForbidden, "Requires manager or higher role")
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
	existing, err := findOne(ctx, colProjMem, bson.M{"org_id": orgID, "project_id": projectID, "user_id": b.UserID}, nil)
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
	if err := insertDoc(ctx, colProjMem, bson.M{
		"org_id": orgID, "project_id": projectID, "user_id": b.UserID,
		"role": role, "added_at": isoNow(), "added_by": user["user_id"],
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, bson.M{"ok": true})
}

func removeProjectMember(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	projectID := c.Param("project_id")
	targetUserID := c.Param("user_id")
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin", "manager") {
		return echo.NewHTTPError(http.StatusForbidden, "Requires manager or higher role")
	}
	_, err := colProjMem.DeleteOne(c.Request().Context(),
		bson.M{"org_id": orgID, "project_id": projectID, "user_id": targetUserID})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, bson.M{"ok": true})
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
	q := bson.M{"org_id": orgID}
	if !all {
		q["project_id"] = bson.M{"$in": allowed}
	}
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
				return c.JSON(http.StatusOK, []bson.M{})
			}
		}
		q["project_id"] = projectID
	}
	status := c.QueryParam("status")
	if status != "" {
		q["status"] = status
	}
	sprints, err := findMany(c.Request().Context(), colSprints, q, bson.M{"_id": 0},
		bson.D{{Key: "created_at", Value: -1}}, 500)
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
	m, herr := requireProjectAccess(c, orgID, b.ProjectID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin", "manager") {
		return echo.NewHTTPError(http.StatusForbidden, "Requires manager or higher role")
	}
	doc := bson.M{
		"sprint_id": newID("spr"), "org_id": orgID, "project_id": b.ProjectID,
		"name": b.Name, "goal": b.Goal,
		"start_date": b.StartDate, "end_date": b.EndDate,
		"status": "planned", "created_at": isoNow(),
	}
	if err := insertDoc(c.Request().Context(), colSprints, doc); err != nil {
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
	sprint, err := findOne(ctx, colSprints, bson.M{"sprint_id": sprintID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if sprint == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Sprint not found")
	}
	m, herr := requireProjectAccess(c, orgID, asStr(sprint["project_id"]), asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin", "manager") {
		return echo.NewHTTPError(http.StatusForbidden, "Requires manager or higher role")
	}
	var b sprintUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Status != nil && !vStatus(*b.Status, sprintStatuses) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	updates := bson.M{}
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
	if _, err := colSprints.UpdateOne(ctx, bson.M{"sprint_id": sprintID, "org_id": orgID}, bson.M{"$set": updates}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	updated, err := findOne(ctx, colSprints, bson.M{"sprint_id": sprintID, "org_id": orgID}, bson.M{"_id": 0})
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
	sprint, err := findOne(ctx, colSprints, bson.M{"sprint_id": sprintID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if sprint == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Sprint not found")
	}
	m, herr := requireProjectAccess(c, orgID, asStr(sprint["project_id"]), asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin", "manager") {
		return echo.NewHTTPError(http.StatusForbidden, "Requires manager or higher role")
	}
	_, _ = colTasks.UpdateMany(ctx,
		bson.M{"org_id": orgID, "sprint_id": sprintID},
		bson.M{"$set": bson.M{"sprint_id": nil, "former_sprint_name": sprint["name"]}})
	_, _ = colSprints.DeleteOne(ctx, bson.M{"sprint_id": sprintID, "org_id": orgID})
	return c.JSON(http.StatusOK, bson.M{"ok": true})
}
