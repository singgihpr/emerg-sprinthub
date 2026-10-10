package main

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
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
	q := bson.M{"org_id": orgID}
	if !all {
		q["project_id"] = bson.M{"$in": allowed}
	}
	tasks, err := findMany(c.Request().Context(), colTasks, q, bson.M{"_id": 0},
		bson.D{{Key: "created_at", Value: -1}}, 2000)
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
	doc := bson.M{
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
	if err := insertDoc(ctx, colTasks, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if recurringID != "" {
		base := todayUTC()
		if b.StartDate != nil && *b.StartDate != "" {
			base = *b.StartDate
		}
		baseDate, ok := parseDate(base)
		if !ok {
			baseDate = nowUTC()
		}
		nextStart := advanceDate(baseDate, repeat)
		desc := ""
		if b.Description != nil {
			desc = *b.Description
		}
		_ = insertDoc(ctx, colRecurring, bson.M{
			"recurring_id": recurringID, "org_id": orgID, "project_id": b.ProjectID,
			"title": b.Title, "description": desc,
			"priority": b.Priority, "type": b.Type,
			"assignee_id": b.AssigneeID, "sprint_id": b.SprintID,
			"estimate_hours": estimate,
			"frequency":      repeat, "next_start": nextStart.Format("2006-01-02"),
			"active": true, "created_by": user["user_id"],
			"created_at": isoNow(), "last_spawned_at": nil,
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
	updates := bson.M{}
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
	res, err := colTasks.UpdateOne(ctx, bson.M{"task_id": taskID, "org_id": orgID}, bson.M{"$set": updates})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if res.MatchedCount == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}
	updated, err := findOne(ctx, colTasks, bson.M{"task_id": taskID}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
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
	_, _ = colTasks.DeleteOne(ctx, bson.M{"task_id": taskID, "org_id": orgID})
	_, _ = colEntries.DeleteMany(ctx, bson.M{"task_id": taskID})
	return c.JSON(http.StatusOK, bson.M{"ok": true})
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
	q := bson.M{"org_id": orgID, "active": true}
	if !all {
		q["project_id"] = bson.M{"$in": allowed}
	}
	rows, err := findMany(c.Request().Context(), colRecurring, q, bson.M{"_id": 0}, nil, 1000)
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
	tpl, err := findOne(ctx, colRecurring, bson.M{"recurring_id": recurringID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if tpl == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Recurring schedule not found")
	}
	if _, herr := requireProjectAccess(c, orgID, asStr(tpl["project_id"]), asStr(user["user_id"])); herr != nil {
		return herr
	}
	if _, err := colRecurring.UpdateOne(ctx, bson.M{"recurring_id": recurringID, "org_id": orgID},
		bson.M{"$set": bson.M{"active": false}}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, bson.M{"status": "stopped"})
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
	allowed, all, herr := accessibleProjectIDs(c, orgID, asStr(user["user_id"]), asStr(m["role"]))
	if herr != nil {
		return herr
	}
	q := bson.M{"org_id": orgID}
	if !all {
		tasks, err := findMany(c.Request().Context(), colTasks,
			bson.M{"org_id": orgID, "project_id": bson.M{"$in": allowed}}, bson.M{"_id": 0}, nil, 5000)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		taskIDs := make([]string, 0, len(tasks))
		for _, t := range tasks {
			taskIDs = append(taskIDs, asStr(t["task_id"]))
		}
		q["task_id"] = bson.M{"$in": taskIDs}
	}
	entries, err := findMany(c.Request().Context(), colEntries, q, bson.M{"_id": 0},
		bson.D{{Key: "created_at", Value: -1}}, 2000)
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
	doc := bson.M{
		"entry_id": newID("te"), "org_id": orgID,
		"task_id": b.TaskID, "user_id": user["user_id"],
		"minutes": b.Minutes, "note": b.Note,
		"date":       date,
		"created_at": isoNow(),
	}
	if err := insertDoc(ctx, colEntries, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	_, _ = colTasks.UpdateOne(ctx, bson.M{"task_id": b.TaskID, "org_id": orgID},
		bson.M{"$inc": bson.M{"logged_minutes": b.Minutes}})
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
	t, err := findOne(c.Request().Context(), colTimers,
		bson.M{"org_id": orgID, "user_id": user["user_id"]}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if t == nil {
		return c.JSON(http.StatusOK, bson.M{})
	}
	return c.JSON(http.StatusOK, t)
}

func startTimer(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	var body bson.M
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
	_, _ = colTimers.DeleteMany(ctx, bson.M{"org_id": orgID, "user_id": user["user_id"]})
	doc := bson.M{
		"timer_id": newID("tmr"), "org_id": orgID,
		"user_id": user["user_id"], "task_id": taskID,
		"started_at": isoNow(),
	}
	if err := insertDoc(ctx, colTimers, doc); err != nil {
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
	t, err := findOne(ctx, colTimers, bson.M{"org_id": orgID, "user_id": user["user_id"]}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if t == nil {
		return echo.NewHTTPError(http.StatusNotFound, "No active timer")
	}
	started, ok := parseISO(asStr(t["started_at"]))
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	minutes := int(nowUTC().Sub(started).Seconds() / 60)
	if minutes < 1 {
		minutes = 1
	}
	entry := bson.M{
		"entry_id": newID("te"), "org_id": orgID,
		"task_id": t["task_id"], "user_id": user["user_id"],
		"minutes": minutes, "note": "Timer",
		"date":       todayUTC(),
		"created_at": isoNow(),
	}
	if err := insertDoc(ctx, colEntries, entry); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	_, _ = colTasks.UpdateOne(ctx, bson.M{"task_id": t["task_id"], "org_id": orgID},
		bson.M{"$inc": bson.M{"logged_minutes": minutes}})
	_, _ = colTimers.DeleteMany(ctx, bson.M{"org_id": orgID, "user_id": user["user_id"]})
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
