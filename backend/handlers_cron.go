package main

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// cron endpoints ack 2xx immediately; the work runs in a goroutine.
func cronAuth(c echo.Context) *echo.HTTPError {
	auth := c.Request().Header.Get("Authorization")
	if cfg.WebhookCronSecret == "" || !strings.HasPrefix(auth, "Bearer ") || auth[7:] != cfg.WebhookCronSecret {
		return echo.NewHTTPError(http.StatusUnauthorized, "Unauthorized")
	}
	return nil
}

func cronEndpoint(name string, work func()) echo.HandlerFunc {
	return func(c echo.Context) error {
		if herr := cronAuth(c); herr != nil {
			return herr
		}
		runID := c.Request().Header.Get("X-Webhook-Id")
		if runID == "" {
			runID = localRunID()
		}
		ctx := context.Background()
		dup, err := pgPoolFindOne(ctx, "cron_runs", map[string]any{"run_id": runID})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if dup != nil {
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "duplicate": true})
		}
		if err := pgPoolInsert(ctx, "cron_runs", map[string]any{"run_id": runID, "name": name}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		go work()
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "queued": true})
	}
}

func cronWeeklyDigest(c echo.Context) error {
	return cronEndpoint("weekly-digest", buildAndSendDigest)(c)
}

func cronSpawnRecurring(c echo.Context) error {
	return cronEndpoint("spawn-recurring", spawnRecurring)(c)
}

// spawnRecurring spawns due recurring task instances once a day.
func spawnRecurring() {
	ctx := context.Background()
	today := nowUTC().Format("2006-01-02")
	templates, err := pgPoolFind(ctx, "recurring_tasks", map[string]any{"active": true})
	if err != nil {
		log.Println("spawn_recurring fetch failed:", err)
		return
	}
	for _, tpl := range templates {
		freq := asStr(tpl["repeat"])
		if freq == "" || freq == "none" {
			continue
		}
		createdStr := asStr(tpl["created_at"])
		createdDate, ok := parseDate(first10(createdStr))
		if !ok {
			createdDate = nowUTC()
		}
		guard := 0
		nextStart := createdDate
		for nextStart.Format("2006-01-02") <= today && guard < 366 {
			guard++
			doc := map[string]any{
				"task_id": newID("tsk"), "org_id": tpl["org_id"],
				"project_id": tpl["project_id"], "title": tpl["title"],
				"description": mapStrDefault(tpl, "description", ""), "status": "todo",
				"priority": mapStrDefault(tpl, "priority", "medium"), "type": mapStrDefault(tpl, "type", "routine"),
				"assignee_id": tpl["assignee_id"], "sprint_id": tpl["sprint_id"],
				"start_date": nextStart.Format("2006-01-02"), "due_date": nextStart.Format("2006-01-02"),
				"estimate_hours": asFloat(tpl["estimate_hours"]), "logged_minutes": 0,
				"recurring_id": tpl["recurring_id"], "repeat": freq,
				"created_by": tpl["created_by"], "created_at": isoNow(),
				"updated_at": isoNow(), "completed_at": nil,
			}
			if err := pgPoolInsert(ctx, "tasks", doc); err != nil {
				log.Println("spawn_recurring insert failed:", err)
				return
			}
			nextStart = advanceDate(nextStart, freq)
		}
		_ = pgPoolUpdate(ctx, "recurring_tasks",
			map[string]any{"recurring_id": tpl["recurring_id"]},
			map[string]any{})
	}
}
