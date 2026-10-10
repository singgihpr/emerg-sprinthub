package main

import (
	"context"
	"time"
)

// Built-in scheduler replacing .emergent/crons.yml: weekly-digest Mondays
// 08:00 UTC, spawn-recurring daily 00:10 UTC. The /api/cron/* webhook
// endpoints remain available for external schedulers.
func startScheduler() {
	go scheduleLoop(nextWeeklyDigest, func() { fireCron("weekly-digest", buildAndSendDigest) })
	go scheduleLoop(nextDailySpawn, func() { fireCron("spawn-recurring", spawnRecurring) })
}

func scheduleLoop(next func(time.Time) time.Time, fire func()) {
	for {
		now := time.Now().UTC()
		target := next(now)
		time.Sleep(target.Sub(now))
		fire()
		time.Sleep(time.Minute) // clear the fired minute before recomputing
	}
}

func nextAt(now time.Time, weekday *time.Weekday, hour, min int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
	for {
		if t.After(now) && (weekday == nil || t.Weekday() == *weekday) {
			return t
		}
		t = t.AddDate(0, 0, 1)
	}
}

func nextWeeklyDigest(now time.Time) time.Time {
	monday := time.Monday
	return nextAt(now, &monday, 8, 0)
}

func nextDailySpawn(now time.Time) time.Time {
	return nextAt(now, nil, 0, 10)
}

// fireCron dedupes on a date-keyed run id so a restart within the same day
// does not double-send the digest.
func fireCron(name string, work func()) {
	runID := name + "-" + todayUTC()
	ctx := context.Background()
	dup, err := pgPoolFindOne(ctx, "cron_runs", map[string]any{"run_id": runID})
	if err != nil || dup != nil {
		return
	}
	if err := pgPoolInsert(ctx, "cron_runs", map[string]any{"run_id": runID, "name": name}); err != nil {
		return
	}
	work()
}
