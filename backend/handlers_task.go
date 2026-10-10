package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// ---- tasks ----

func listTasks(c echo.Context) error {
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
	var tasks []map[string]any
	var err error
	if all {
		tasks, err = pgFindMany(ctx, "tasks", map[string]any{"org_id": orgID}, nil, "created_at DESC", 2000)
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
		q := fmt.Sprintf("SELECT * FROM tasks WHERE org_id = $1 AND project_id IN (%s) ORDER BY created_at DESC LIMIT 2000",
			strings.Join(placeholders, ","))
		rows, e := pgExecQuery(ctx, q, args...)
		if e != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		tasks = rows
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, tasks)
}

func createTask(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	var b taskCreate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.ProjectID == "" || b.Title == "" ||
		(b.Repeat != nil && !vStatus(*b.Repeat, repeatModes)) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	if _, herr := requireProjectAccess(c, orgID, b.ProjectID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	repeat := "none"
	if b.Repeat != nil && *b.Repeat != "" {
		repeat = *b.Repeat
	}
	recurringID := ""
	if repeat == "daily" || repeat == "weekly" || repeat == "monthly" {
		recurringID = newID("rec")
	}
	ctx := c.Request().Context()
	estimate := 0.0
	if b.EstimateHours != nil {
		estimate = *b.EstimateHours
	}
	doc := map[string]any{
		"task_id": newID("tsk"), "org_id": orgID,
		"project_id": b.ProjectID, "title": b.Title,
		"description": b.Description, "status": b.Status,
		"priority": b.Priority, "type": b.Type,
		"assignee_id": b.AssigneeID, "sprint_id": b.SprintID,
		"start_date": b.StartDate, "due_date": b.DueDate,
		"estimate_hours": estimate,
		"logged_minutes": 0,
		"recurring_id":   recurringID, "repeat": repeat,
		"created_by": user["user_id"], "created_at": isoNow(),
		"updated_at":   isoNow(),
		"completed_at": nil,
	}
	if err := pgInsert(ctx, "tasks", doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if recurringID != "" {
		desc := ""
		if b.Description != nil {
			desc = *b.Description
		}
		_ = pgInsert(ctx, "recurring_tasks", map[string]any{
			"recurring_id": recurringID, "org_id": orgID, "project_id": b.ProjectID,
			"title": b.Title, "description": desc,
			"priority": b.Priority, "type": b.Type,
			"assignee_id": b.AssigneeID, "sprint_id": b.SprintID,
			"estimate_hours": estimate,
			"repeat": repeat, "active": true, "created_by": user["user_id"],
			"created_at": isoNow(),
		})
	}
	return c.JSON(http.StatusOK, doc)
}

func updateTask(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	taskID := c.Param("task_id")
	if _, herr := requireTaskAccess(c, orgID, taskID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	var b taskUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	updates := map[string]any{}
	if b.Title != nil {
		updates["title"] = *b.Title
	}
	if b.Description != nil {
		updates["description"] = *b.Description
	}
	if b.Status != nil {
		updates["status"] = *b.Status
	}
	if b.Priority != nil {
		updates["priority"] = *b.Priority
	}
	if b.AssigneeID != nil {
		updates["assignee_id"] = *b.AssigneeID
	}
	if b.SprintID != nil {
		updates["sprint_id"] = *b.SprintID
	}
	if b.StartDate != nil {
		updates["start_date"] = *b.StartDate
	}
	if b.DueDate != nil {
		updates["due_date"] = *b.DueDate
	}
	if b.EstimateHours != nil {
		updates["estimate_hours"] = *b.EstimateHours
	}
	updates["updated_at"] = isoNow()
	if s, ok := updates["status"].(string); ok && s == "done" {
		updates["completed_at"] = isoNow()
	}
	if s, ok := updates["sprint_id"].(string); ok && s != "" {
		updates["former_sprint_name"] = nil
	}
	ctx := c.Request().Context()
	if err := pgUpdate(ctx, "tasks", map[string]any{"task_id": taskID, "org_id": orgID}, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	updated, err := pgFindOne(ctx, "tasks", map[string]any{"task_id": taskID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if updated == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}
	return c.JSON(http.StatusOK, updated)
}

func deleteTask(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	taskID := c.Param("task_id")
	if _, herr := requireTaskAccess(c, orgID, taskID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	_ = pgDelete(ctx, "tasks", map[string]any{"task_id": taskID, "org_id": orgID})
	_ = pgDelete(ctx, "time_entries", map[string]any{"task_id": taskID})
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// ---- recurring ----

func listRecurring(c echo.Context) error {
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
	var rows []map[string]any
	var err error
	if all {
		rows, err = pgFindMany(ctx, "recurring_tasks", map[string]any{"org_id": orgID, "active": true}, nil, "", 1000)
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
		q := fmt.Sprintf("SELECT * FROM recurring_tasks WHERE org_id = $1 AND active = true AND project_id IN (%s) LIMIT 1000",
			strings.Join(placeholders, ","))
		result, e := pgExecQuery(ctx, q, args...)
		if e != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		rows = result
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, rows)
}

func stopRecurring(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	recurringID := c.Param("recurring_id")
	ctx := c.Request().Context()
	tpl, err := pgFindOne(ctx, "recurring_tasks", map[string]any{"recurring_id": recurringID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if tpl == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Recurring schedule not found")
	}
	if _, herr := requireProjectAccess(c, orgID, asStr(tpl["project_id"]), asStr(user["user_id"])); herr != nil {
		return herr
	}
	if err := pgUpdate(ctx, "recurring_tasks", map[string]any{"recurring_id": recurringID, "org_id": orgID},
		map[string]any{"active": false}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "stopped"})
}

// ---- time tracking ----

func listTimeEntries(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	_, all, herr := accessibleProjectIDs(c, orgID, asStr(user["user_id"]), asStr(m["role"]))
	if herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	var entries []map[string]any
	var err error
	if all {
		entries, err = pgFindMany(ctx, "time_entries", map[string]any{"org_id": orgID}, nil, "created_at DESC", 2000)
	} else {
		tasks, err := pgFindMany(ctx, "tasks",
			map[string]any{"org_id": orgID}, []string{"task_id"}, "", 5000)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		taskIDs := make([]string, 0, len(tasks))
		for _, t := range tasks {
			taskIDs = append(taskIDs, asStr(t["task_id"]))
		}
		if len(taskIDs) == 0 {
			return c.JSON(http.StatusOK, []map[string]any{})
		}
		placeholders := make([]string, len(taskIDs))
		args := []any{orgID}
		for i, id := range taskIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, id)
		}
		q := fmt.Sprintf("SELECT * FROM time_entries WHERE org_id = $1 AND task_id IN (%s) ORDER BY created_at DESC LIMIT 2000",
			strings.Join(placeholders, ","))
		rows, e := pgExecQuery(ctx, q, args...)
		if e != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		entries = rows
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, entries)
}

func createTimeEntry(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	var b timeEntryCreate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.TaskID == "" || b.Minutes == 0 {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	if _, herr := requireTaskAccess(c, orgID, b.TaskID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	date := todayUTC()
	if b.Date != nil && *b.Date != "" {
		date = *b.Date
	}
	doc := map[string]any{
		"entry_id": newID("te"), "org_id": orgID,
		"task_id": b.TaskID, "user_id": user["user_id"],
		"minutes": b.Minutes, "note": b.Note,
		"date":       date,
		"created_at": isoNow(),
	}
	if err := pgInsert(ctx, "time_entries", doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	// Increment task logged_minutes
	task, _ := pgFindOne(ctx, "tasks", map[string]any{"task_id": b.TaskID, "org_id": orgID}, nil)
	if task != nil {
		logged := 0
		if v, ok := task["logged_minutes"].(int64); ok {
			logged = int(v)
		} else if v, ok := task["logged_minutes"].(int); ok {
			logged = v
		}
		_ = pgUpdate(ctx, "tasks", map[string]any{"task_id": b.TaskID, "org_id": orgID},
			map[string]any{"logged_minutes": logged + b.Minutes})
	}
	return c.JSON(http.StatusOK, doc)
}

// ---- active timer ----

func getTimer(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if _, herr := ensureMember(c, orgID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	t, err := pgFindOne(c.Request().Context(), "active_timers",
		map[string]any{"org_id": orgID, "user_id": user["user_id"]}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if t == nil {
		return c.JSON(http.StatusOK, map[string]any{})
	}
	return c.JSON(http.StatusOK, t)
}

func startTimer(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	var body map[string]any
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	taskID := asStr(body["task_id"])
	if taskID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "task_id required")
	}
	if _, herr := requireTaskAccess(c, orgID, taskID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	_ = pgDelete(ctx, "active_timers", map[string]any{"org_id": orgID, "user_id": user["user_id"]})
	doc := map[string]any{
		"org_id": orgID,
		"user_id": user["user_id"], "task_id": taskID,
		"started_at": isoNow(),
	}
	if err := pgInsert(ctx, "active_timers", doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, doc)
}

func stopTimer(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if _, herr := ensureMember(c, orgID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	t, err := pgFindOne(ctx, "active_timers", map[string]any{"org_id": orgID, "user_id": user["user_id"]}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if t == nil {
		return echo.NewHTTPError(http.StatusNotFound, "No active timer")
	}
	var started time.Time
	var ok bool
	switch v := t["started_at"].(type) {
	case time.Time:
		started = v
		ok = true
	case string:
		started, ok = parseISO(v)
	}
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	minutes := int(nowUTC().Sub(started).Seconds() / 60)
	if minutes < 1 {
		minutes = 1
	}
	entry := map[string]any{
		"entry_id": newID("te"), "org_id": orgID,
		"task_id": t["task_id"], "user_id": user["user_id"],
		"minutes": minutes, "note": "Timer",
		"date":       todayUTC(),
		"created_at": isoNow(),
	}
	if err := pgInsert(ctx, "time_entries", entry); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	task, _ := pgFindOne(ctx, "tasks", map[string]any{"task_id": t["task_id"], "org_id": orgID}, nil)
	if task != nil {
		logged := 0
		if v, ok := task["logged_minutes"].(int64); ok {
			logged = int(v)
		} else if v, ok := task["logged_minutes"].(int); ok {
			logged = v
		}
		_ = pgUpdate(ctx, "tasks", map[string]any{"task_id": t["task_id"], "org_id": orgID},
			map[string]any{"logged_minutes": logged + minutes})
	}
	_ = pgDelete(ctx, "active_timers", map[string]any{"org_id": orgID, "user_id": user["user_id"]})
	return c.JSON(http.StatusOK, entry)
}

// ---- shared date math ----

func advanceDate(d time.Time, freq string) time.Time {
	switch freq {
	case "weekly":
		return d.AddDate(0, 0, 7)
	case "monthly":
		y, mo := d.Year(), int(d.Month())+1
		if mo == 13 {
			y, mo = y+1, 1
		}
		last := time.Date(y, time.Month(mo)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		dd := d.Day()
		if dd > last {
			dd = last
		}
		return time.Date(y, time.Month(mo), dd, 0, 0, 0, 0, time.UTC)
	default:
		return d.AddDate(0, 0, 1)
	}
}
