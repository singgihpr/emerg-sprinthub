package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// ---- team activity (admin/manager only) ----

func teamActivity(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermViewAnalytics); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	ctx := c.Request().Context()

	memberships, err := pgFindMany(ctx, "memberships", map[string]any{"org_id": orgID}, nil, "created_at", 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	userIDs := make([]string, 0, len(memberships))
	for _, x := range memberships {
		userIDs = append(userIDs, asStr(x["user_id"]))
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
	roleMap := map[string]any{}
	for _, x := range memberships {
		roleMap[asStr(x["user_id"])] = x["role"]
	}

	tasks, err := pgFindMany(ctx, "tasks", map[string]any{"org_id": orgID}, nil, "", 5000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	activeTimers, err := pgFindMany(ctx, "active_timers", map[string]any{"org_id": orgID}, nil, "", 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	timerMap := map[string]any{}
	for _, t := range activeTimers {
		timerMap[asStr(t["user_id"])] = t
	}
	taskMap := map[string]any{}
	for _, t := range tasks {
		taskMap[asStr(t["task_id"])] = t
	}
	projects, err := pgFindMany(ctx, "projects", map[string]any{"org_id": orgID}, nil, "", 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	projectMap := map[string]any{}
	for _, p := range projects {
		projectMap[asStr(p["project_id"])] = p
	}

	todayISO := todayUTC()
	weekStart := nowUTC().AddDate(0, 0, -6).Format("2006-01-02")

	entries, err := pgFindMany(ctx, "time_entries", map[string]any{"org_id": orgID}, nil, "created_at DESC", 2000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	// Filter entries by week
	filteredEntries := []map[string]any{}
	for _, e := range entries {
		if asStr(e["date"]) >= weekStart {
			filteredEntries = append(filteredEntries, e)
		}
	}
	entries = filteredEntries
	nowTS := nowUTC()

	rows := make([]map[string]any, 0, len(userIDs))
	for _, uid := range userIDs {
		u := umap[uid]
		if u == nil {
			u = map[string]any{}
		}
		var myTasks, activeTasks, inProgress []map[string]any
		for _, t := range tasks {
			if asStr(t["assignee_id"]) != uid {
				continue
			}
			myTasks = append(myTasks, t)
			if asStr(t["status"]) != "done" {
				activeTasks = append(activeTasks, t)
			}
			if asStr(t["status"]) == "in_progress" {
				inProgress = append(inProgress, t)
			}
		}

		var myEntries []map[string]any
		for _, e := range entries {
			if asStr(e["user_id"]) == uid {
				myEntries = append(myEntries, e)
			}
		}
		todayMin, weekMin := 0, 0
		for _, e := range myEntries {
			weekMin += asInt(e["minutes"])
			if asStr(e["date"]) == todayISO {
				todayMin += asInt(e["minutes"])
			}
		}

		var activeTimer map[string]any
		if tmr, ok := timerMap[uid].(map[string]any); ok {
			started, sOK := parseISO(asStr(tmr["started_at"]))
			elapsed := 0
			if sOK && started.Before(nowTS) {
				elapsed = int(nowTS.Sub(started).Seconds())
			}
			if elapsed < 0 {
				elapsed = 0
			}
			tTask, _ := taskMap[asStr(tmr["task_id"])].(map[string]any)
			taskTitle := "Unknown task"
			var projectKey interface{}
			if tTask != nil {
				taskTitle = asStr(tTask["title"])
				if p, ok := projectMap[asStr(tTask["project_id"])].(map[string]any); ok {
					projectKey = p["key"]
				}
			}
			activeTimer = map[string]any{
				"task_id":         tmr["task_id"],
				"task_title":      taskTitle,
				"project_key":     projectKey,
				"started_at":      tmr["started_at"],
				"elapsed_seconds": elapsed,
			}
		}

		recent := make([]map[string]any, 0, 5)
		for i, e := range myEntries {
			if i >= 5 {
				break
			}
			tTask, _ := taskMap[asStr(e["task_id"])].(map[string]any)
			taskTitle := "Deleted task"
			if tTask != nil {
				taskTitle = asStr(tTask["title"])
			}
			note := ""
			if v, ok := e["note"].(string); ok {
				note = v
			}
			recent = append(recent, map[string]any{
				"task_id":    e["task_id"],
				"task_title": taskTitle,
				"minutes":    e["minutes"],
				"date":       e["date"],
				"note":       note,
				"created_at": e["created_at"],
			})
		}

		estimateHours := 0.0
		for _, t := range activeTasks {
			estimateHours += asFloat(t["estimate_hours"])
		}
		inProg := make([]map[string]any, 0, 5)
		for i, t := range inProgress {
			if i >= 5 {
				break
			}
			var projectKey interface{}
			if p, ok := projectMap[asStr(t["project_id"])].(map[string]any); ok {
				projectKey = p["key"]
			}
			inProg = append(inProg, map[string]any{
				"task_id": t["task_id"], "title": t["title"], "priority": t["priority"],
				"project_key": projectKey,
				"due_date":    t["due_date"],
			})
		}

		rows = append(rows, map[string]any{
			"user_id":              uid,
			"name":                 u["name"],
			"email":                u["email"],
			"picture":              u["picture"],
			"role":                 roleMap[uid],
			"active_timer":         activeTimer,
			"active_count":         len(activeTasks),
			"in_progress_tasks":    inProg,
			"estimate_hours":       estimateHours,
			"logged_today_minutes": todayMin,
			"logged_week_minutes":  weekMin,
			"recent_activity":      recent,
		})
	}

	// sort: active timer first, then by active count desc (python: (timer is None, -count))
	sort.SliceStable(rows, func(i, j int) bool {
		ni := rows[i]["active_timer"] == nil
		nj := rows[j]["active_timer"] == nil
		if ni != nj {
			return nj // non-nil timer sorts first
		}
		return asInt(rows[i]["active_count"]) > asInt(rows[j]["active_count"])
	})
	return c.JSON(http.StatusOK, map[string]any{"generated_at": nowTS.Format("2006-01-02T15:04:05.000000-07:00"), "members": rows})
}

// ---- sprint burndown ----

func sprintBurndown(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	sprintID := c.Param("sprint_id")
	if _, herr := ensureMember(c, orgID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	sprint, err := pgFindOne(ctx, "sprints", map[string]any{"sprint_id": sprintID, "org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if sprint == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Sprint not found")
	}
	tasks, err := pgFindMany(ctx, "tasks", map[string]any{"org_id": orgID, "sprint_id": sprintID}, nil, "", 2000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	totalHours := 0.0
	for _, t := range tasks {
		totalHours += asFloat(t["estimate_hours"])
	}
	startStr, endStr := asStr(sprint["start_date"]), asStr(sprint["end_date"])
	if startStr == "" || endStr == "" {
		return c.JSON(http.StatusOK, map[string]any{
			"sprint_id": sprintID, "total_estimate_hours": totalHours, "series": []map[string]any{},
		})
	}
	startD, sOK := parseDate(first10(startStr))
	endD, eOK := parseDate(first10(endStr))
	if !sOK || !eOK {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	daysTotal := int(endD.Sub(startD).Hours() / 24)
	if daysTotal < 1 {
		daysTotal = 1
	}
	today := todayUTC()

	series := []map[string]any{}
	completedCount := 0
	for cursor := startD; !cursor.After(endD); cursor = cursor.AddDate(0, 0, 1) {
		idx := int(cursor.Sub(startD).Hours() / 24)
		ideal := math.Round(totalHours*(1-float64(idx)/float64(daysTotal))*100) / 100
		completedHours := 0.0
		for _, t := range tasks {
			ca := asStr(t["completed_at"])
			if ca == "" {
				continue
			}
			caDate, ok := parseISO(ca)
			if !ok {
				continue
			}
			if caDate.Format("2006-01-02") <= cursor.Format("2006-01-02") {
				completedHours += asFloat(t["estimate_hours"])
			}
		}
		actual := math.Round((totalHours-completedHours)*100) / 100
		var actualVal interface{} = actual
		if cursor.Format("2006-01-02") > today {
			actualVal = nil
		}
		series = append(series, map[string]any{
			"date":   cursor.Format("2006-01-02"),
			"ideal":  ideal,
			"actual": actualVal,
		})
	}
	for _, t := range tasks {
		if asStr(t["status"]) == "done" {
			completedCount++
		}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"sprint_id":            sprintID,
		"sprint_name":          sprint["name"],
		"start_date":           startStr,
		"end_date":             endStr,
		"total_tasks":          len(tasks),
		"completed_tasks":      completedCount,
		"total_estimate_hours": totalHours,
		"series":               series,
	})
}

// ---- task comments ----

var mentionRe = regexp.MustCompile(`@([\w.+-]+@[\w-]+\.[\w.-]+)`)

func listComments(c echo.Context) error {
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
	comments, err := pgFindMany(ctx, "comments",
		map[string]any{"org_id": orgID, "task_id": taskID}, nil, "created_at ASC", 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	seen := map[string]bool{}
	authorIDs := []string{}
	for _, cm := range comments {
		aid := asStr(cm["user_id"])
		if _, dup := seen[aid]; !dup {
			seen[aid] = true
			authorIDs = append(authorIDs, aid)
		}
	}
	amap := map[string]map[string]any{}
	for _, aid := range authorIDs {
		a, err := pgFindOne(ctx, "users", map[string]any{"user_id": aid}, []string{"user_id", "email", "name", "picture", "created_at"})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if a != nil {
			amap[aid] = a
		}
	}
	for _, cm := range comments {
		a := amap[asStr(cm["user_id"])]
		if a == nil {
			a = map[string]any{}
		}
		cm["author_id"] = cm["user_id"]
		cm["author_name"] = mapStrDefault(a, "name", mapStrDefault(a, "email", ""))
		cm["author_email"] = a["email"]
		cm["author_picture"] = a["picture"]
		// Convert JSONB mentions from DB bytes to []string
		if raw, ok := cm["mentions"]; ok {
			switch v := raw.(type) {
			case []byte:
				var arr []string
				if json.Unmarshal(v, &arr) == nil {
					cm["mentions"] = arr
				}
			case string:
				var arr []string
				if json.Unmarshal([]byte(v), &arr) == nil {
					cm["mentions"] = arr
				}
			}
		} else {
			cm["mentions"] = []string{}
		}
	}
	return c.JSON(http.StatusOK, comments)
}

func createComment(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	taskID := c.Param("task_id")
	if _, herr := requireTaskAccess(c, orgID, taskID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	var b commentCreate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if strings.TrimSpace(b.Body) == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Comment cannot be empty")
	}
	ctx := c.Request().Context()
	seen := map[string]bool{}
	mentions := []string{}
	for _, m := range mentionRe.FindAllStringSubmatch(b.Body, -1) {
		low := strings.ToLower(m[1])
		if _, dup := seen[low]; !dup {
			seen[low] = true
			mentions = append(mentions, low)
		}
	}
	mentionedUsers := []string{}
	for _, email := range mentions {
		u, err := pgFindOne(ctx, "users", map[string]any{"email": email}, []string{"user_id"})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if u != nil {
			mentionedUsers = append(mentionedUsers, asStr(u["user_id"]))
		}
	}
	mentionsJSON, _ := json.Marshal(mentionedUsers)
	doc := map[string]any{
		"comment_id": newID("cmt"), "org_id": orgID, "task_id": taskID,
		"user_id": user["user_id"], "body": b.Body,
		"mentions": mentionsJSON,
		"created_at": isoNow(),
	}
	if err := pgInsert(ctx, "comments", doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	doc["author_id"] = doc["user_id"]
	doc["author_name"] = mapStrDefault(user, "name", mapStrDefault(user, "email", ""))
	doc["author_email"] = user["email"]
	doc["author_picture"] = user["picture"]
	doc["mentions"] = mentionedUsers
	return c.JSON(http.StatusOK, doc)
}

// ---- analytics ----

func analytics(c echo.Context) error {
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
		tasks, err = pgFindMany(ctx, "tasks", map[string]any{"org_id": orgID}, nil, "", 5000)
	} else {
		if len(allowed) == 0 {
			tasks = []map[string]any{}
		} else {
			q := fmt.Sprintf("SELECT * FROM tasks WHERE org_id = $1 AND project_id IN (%s) LIMIT 5000",
				inPlaceholders(len(allowed), 2))
			args := append([]any{orgID}, toAnySlice(allowed)...)
			tasks, err = pgExecQuery(ctx, q, args...)
		}
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	accessibleTaskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		accessibleTaskIDs = append(accessibleTaskIDs, asStr(t["task_id"]))
	}
	var entries []map[string]any
	if all {
		entries, err = pgFindMany(ctx, "time_entries", map[string]any{"org_id": orgID}, nil, "", 5000)
	} else {
		if len(accessibleTaskIDs) == 0 {
			entries = []map[string]any{}
		} else {
			q := fmt.Sprintf("SELECT * FROM time_entries WHERE org_id = $1 AND task_id IN (%s) LIMIT 5000",
				inPlaceholders(len(accessibleTaskIDs), 2))
			args := append([]any{orgID}, toAnySlice(accessibleTaskIDs)...)
			entries, err = pgExecQuery(ctx, q, args...)
		}
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}

	today := nowUTC()
	startQ, endQ := c.QueryParam("start"), c.QueryParam("end")
	periodMode := startQ != "" && endQ != ""
	var dateList []string
	var periodTasks, scopedEntries []map[string]any
	completedTasks := 0

	if periodMode {
		startDate, sOK := parseDateFlexible(startQ)
		endDate, eOK := parseDateFlexible(endQ)
		if !sOK || !eOK {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "Invalid date format, use YYYY-MM-DD")
		}
		if startDate.After(endDate) {
			startDate, endDate = endDate, startDate
		}
		if int(endDate.Sub(startDate).Hours()/24) > 366 {
			startDate = endDate.AddDate(0, 0, -366)
		}
		startISO, endISO := startDate.Format("2006-01-02"), endDate.Format("2006-01-02")
		numDays := int(endDate.Sub(startDate).Hours()/24) + 1
		dateList = make([]string, 0, numDays)
		for i := 0; i < numDays; i++ {
			dateList = append(dateList, startDate.AddDate(0, 0, i).Format("2006-01-02"))
		}
		for _, t := range tasks {
			d := first10(asStr(t["created_at"]))
			if startISO <= d && d <= endISO {
				periodTasks = append(periodTasks, t)
			}
			d = first10(asStr(t["completed_at"]))
			if startISO <= d && d <= endISO {
				completedTasks++
			}
		}
		if periodTasks == nil {
			periodTasks = []map[string]any{}
		}
		for _, e := range entries {
			d := asStr(e["date"])
			if startISO <= d && d <= endISO {
				scopedEntries = append(scopedEntries, e)
			}
		}
	} else {
		for i := 6; i >= 0; i-- {
			dateList = append(dateList, today.AddDate(0, 0, -i).Format("2006-01-02"))
		}
		periodTasks = tasks
		scopedEntries = entries
		for _, t := range tasks {
			if asStr(t["status"]) == "done" {
				completedTasks++
			}
		}
	}

	byStatus := map[string]any{"todo": 0, "in_progress": 0, "review": 0, "done": 0}
	for _, t := range periodTasks {
		s := mapStrDefault(t, "status", "todo")
		byStatus[s] = asInt(byStatus[s]) + 1
	}

	completedSeries := make([]map[string]any, 0, len(dateList))
	for _, d := range dateList {
		cnt := 0
		for _, t := range tasks {
			if strings.HasPrefix(asStr(t["completed_at"]), d) {
				cnt++
			}
		}
		completedSeries = append(completedSeries, map[string]any{"date": d, "completed": cnt})
	}
	timeSeries := make([]map[string]any, 0, len(dateList))
	for _, d := range dateList {
		mins := 0
		for _, e := range scopedEntries {
			if asStr(e["date"]) == d {
				mins += asInt(e["minutes"])
			}
		}
		timeSeries = append(timeSeries, map[string]any{"date": d, "minutes": mins})
	}

	totalEstimate := 0.0
	for _, t := range periodTasks {
		totalEstimate += asFloat(t["estimate_hours"]) * 60
	}
	totalLogged := 0
	for _, e := range scopedEntries {
		totalLogged += asInt(e["minutes"])
	}
	totalTasks := len(periodTasks)
	completionRate := 0.0
	if totalTasks > 0 {
		completionRate = float64(completedTasks) / float64(totalTasks) * 100
	}
	return c.JSON(http.StatusOK, map[string]any{
		"by_status":              byStatus,
		"total_tasks":            totalTasks,
		"completed_tasks":        completedTasks,
		"completion_rate":        completionRate,
		"total_logged_minutes":   totalLogged,
		"total_estimate_minutes": totalEstimate,
		"completed_series":       completedSeries,
		"time_series":            timeSeries,
	})
}

// parseDateFlexible mirrors python datetime.fromisoformat(...).date()
func parseDateFlexible(s string) (time.Time, bool) {
	if t, ok := parseDate(s); ok {
		return t, true
	}
	if t, ok := parseISO(s); ok {
		return t, true
	}
	return time.Time{}, false
}

func mapStrDefault(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func inPlaceholders(n, start int) string {
	ph := make([]string, n)
	for i := 0; i < n; i++ {
		ph[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(ph, ",")
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
