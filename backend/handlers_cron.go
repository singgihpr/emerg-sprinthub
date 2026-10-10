package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
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
		ctx := c.Request().Context()
		dup, err := findOne(ctx, colCronRuns, bson.M{"run_id": runID}, nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if dup != nil {
			return c.JSON(http.StatusOK, bson.M{"ok": true, "duplicate": true})
		}
		if err := insertDoc(ctx, colCronRuns, bson.M{"run_id": runID, "at": isoNow(), "name": name}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		go work()
		return c.JSON(http.StatusOK, bson.M{"ok": true, "queued": true})
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
	ctx := mongoCtx()
	today := nowUTC().Format("2006-01-02")
	templates, err := findMany(ctx, colRecurring, bson.M{"active": true}, nil, nil, 1000)
	if err != nil {
		log.Println("spawn_recurring fetch failed:", err)
		return
	}
	for _, tpl := range templates {
		nextStart, ok := parseDate(asStr(tpl["next_start"]))
		if !ok {
			continue
		}
		freq := asStr(tpl["frequency"])
		guard := 0
		for nextStart.Format("2006-01-02") <= today && guard < 366 {
			guard++
			doc := bson.M{
				"task_id": newID("tsk"), "org_id": tpl["org_id"],
				"project_id": tpl["project_id"], "title": tpl["title"],
				"description": bsonStr(tpl, "description", ""), "status": "todo",
				"priority": bsonStr(tpl, "priority", "medium"), "type": bsonStr(tpl, "type", "routine"),
				"assignee_id": tpl["assignee_id"], "sprint_id": tpl["sprint_id"],
				"start_date": nextStart.Format("2006-01-02"), "due_date": nextStart.Format("2006-01-02"),
				"estimate_hours": asFloat(tpl["estimate_hours"]), "logged_minutes": 0,
				"recurring_id": tpl["recurring_id"], "repeat": freq,
				"created_by": tpl["created_by"], "created_at": isoNow(),
				"updated_at": isoNow(), "completed_at": nil,
			}
			if err := insertDoc(ctx, colTasks, doc); err != nil {
				log.Println("spawn_recurring insert failed:", err)
				return
			}
			nextStart = advanceDate(nextStart, freq)
		}
		_, _ = colRecurring.UpdateOne(ctx,
			bson.M{"recurring_id": tpl["recurring_id"]},
			bson.M{"$set": bson.M{"next_start": nextStart.Format("2006-01-02"), "last_spawned_at": isoNow()}})
	}
}
