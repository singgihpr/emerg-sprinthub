package main

import (
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
)

// ---- team activity (admin/manager only) ----

func teamActivity(c echo.Context) error {
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
		return echo.NewHTTPError(http.StatusForbidden, "Requires admin or manager role")
	}
	ctx := c.Request().Context()

	memberships, err := findMany(ctx, colMembers, bson.M{"org_id": orgID}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	userIDs := make([]string, 0, len(memberships))
	for _, x := range memberships {
		userIDs = append(userIDs, asStr(x["user_id"]))
	}
	users, err := findMany(ctx, colUsers, bson.M{"user_id": bson.M{"$in": userIDs}}, bson.M{"_id": 0, "password_hash": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	umap := bson.M{}
	for _, u := range users {
		umap[asStr(u["user_id"])] = u
	}
	roleMap := bson.M{}
	for _, x := range memberships {
		roleMap[asStr(x["user_id"])] = x["role"]
	}

	tasks, err := findMany(ctx, colTasks, bson.M{"org_id": orgID}, bson.M{"_id": 0}, nil, 5000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	activeTimers, err := findMany(ctx, colTimers, bson.M{"org_id": orgID}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	timerMap := bson.M{}
	for _, t := range activeTimers {
		timerMap[asStr(t["user_id"])] = t
	}
	taskMap := bson.M{}
	for _, t := range tasks {
		taskMap[asStr(t["task_id"])] = t
	}
	projects, err := findMany(ctx, colProjects, bson.M{"org_id": orgID}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	projectMap := bson.M{}
	for _, p := range projects {
		projectMap[asStr(p["project_id"])] = p
	}

	todayISO := todayUTC()
	weekStart := nowUTC().AddDate(0, 0, -6).Format("2006-01-02")

	entries, err := findMany(ctx, colEntries,
		bson.M{"org_id": orgID, "date": bson.M{"$gte": weekStart}},
		bson.M{"_id": 0}, bson.D{{Key: "created_at", Value: -1}}, 2000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	nowTS := nowUTC()

	rows := make([]bson.M, 0, len(userIDs))
	for _, uid := range userIDs {
		u, _ := umap[uid].(bson.M)
		if u == nil {
			u = bson.M{}
		}
		var myTasks, activeTasks, inProgress []bson.M
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

		var myEntries []bson.M
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

		var activeTimer bson.M
		if tmr, ok := timerMap[uid].(bson.M); ok {
			started, sOK := parseISO(asStr(tmr["started_at"]))
			elapsed := 0
			if sOK && started.Before(nowTS) {
				elapsed = int(nowTS.Sub(started).Seconds())
			}
			if elapsed < 0 {
				elapsed = 0
			}
			tTask, _ := taskMap[asStr(tmr["task_id"])].(bson.M)
			taskTitle := "Unknown task"
			var projectKey interface{}
			if tTask != nil {
				taskTitle = asStr(tTask["title"])
				if p, ok := projectMap[asStr(tTask["project_id"])].(bson.M); ok {
					projectKey = p["key"]
				}
			}
			activeTimer = bson.M{
				"task_id":         tmr["task_id"],
				"task_title":      taskTitle,
				"project_key":     projectKey,
				"started_at":      tmr["started_at"],
				"elapsed_seconds": elapsed,
			}
		}

		recent := make([]bson.M, 0, 5)
		for i, e := range myEntries {
			if i >= 5 {
				break
			}
			tTask, _ := taskMap[asStr(e["task_id"])].(bson.M)
			taskTitle := "Deleted task"
			if tTask != nil {
				taskTitle = asStr(tTask["title"])
			}
			recent = append(recent, bson.M{
				"task_id":    e["task_id"],
				"task_title": taskTitle,
				"minutes":    e["minutes"],
				"date":       e["date"],
				"note":       bsonStr(e, "note", ""),
				"created_at": e["created_at"],
			})
		}

		estimateHours := 0.0
		for _, t := range activeTasks {
			estimateHours += asFloat(t["estimate_hours"])
		}
		inProg := make([]bson.M, 0, 5)
		for i, t := range inProgress {
			if i >= 5 {
				break
			}
			var projectKey interface{}
			if p, ok := projectMap[asStr(t["project_id"])].(bson.M); ok {
				projectKey = p["key"]
			}
			inProg = append(inProg, bson.M{
				"task_id": t["task_id"], "title": t["title"], "priority": t["priority"],
				"project_key": projectKey,
				"due_date":    t["due_date"],
			})
		}

		rows = append(rows, bson.M{
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
	return c.JSON(http.StatusOK, bson.M{"generated_at": nowTS.Format("2006-01-02T15:04:05.000000-07:00"), "members": rows})
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
	sprint, err := findOne(ctx, colSprints, bson.M{"sprint_id": sprintID, "org_id": orgID}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if sprint == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Sprint not found")
	}
	tasks, err := findMany(ctx, colTasks, bson.M{"org_id": orgID, "sprint_id": sprintID}, bson.M{"_id": 0}, nil, 2000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	totalHours := 0.0
	for _, t := range tasks {
		totalHours += asFloat(t["estimate_hours"])
	}
	startStr, endStr := asStr(sprint["start_date"]), asStr(sprint["end_date"])
	if startStr == "" || endStr == "" {
		return c.JSON(http.StatusOK, bson.M{
			"sprint_id": sprintID, "total_estimate_hours": totalHours, "series": []bson.M{},
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

	series := []bson.M{}
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
		series = append(series, bson.M{
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
	return c.JSON(http.StatusOK, bson.M{
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
	comments, err := findMany(ctx, colComments,
		bson.M{"org_id": orgID, "task_id": taskID}, bson.M{"_id": 0},
		bson.D{{Key: "created_at", Value: 1}}, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	seen := bson.M{}
	authorIDs := []string{}
	for _, cm := range comments {
		aid := asStr(cm["author_id"])
		if _, dup := seen[aid]; !dup {
			seen[aid] = true
			authorIDs = append(authorIDs, aid)
		}
	}
	authors, err := findMany(ctx, colUsers, bson.M{"user_id": bson.M{"$in": authorIDs}}, bson.M{"_id": 0, "password_hash": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	amap := bson.M{}
	for _, a := range authors {
		amap[asStr(a["user_id"])] = a
	}
	for _, cm := range comments {
		a, _ := amap[asStr(cm["author_id"])].(bson.M)
		if a == nil {
			a = bson.M{}
		}
		cm["author_name"] = bsonStr(a, "name", bsonStr(a, "email", ""))
		cm["author_email"] = a["email"]
		cm["author_picture"] = a["picture"]
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
	seen := bson.M{}
	mentions := []string{}
	for _, m := range mentionRe.FindAllStringSubmatch(b.Body, -1) {
		low := strings.ToLower(m[1])
		if _, dup := seen[low]; !dup {
			seen[low] = true
			mentions = append(mentions, low)
		}
	}
	mentionedUsers := []string{}
	if len(mentions) > 0 {
		found, err := findMany(ctx, colUsers, bson.M{"email": bson.M{"$in": mentions}}, bson.M{"_id": 0, "password_hash": 0}, nil, 100)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		for _, u := range found {
			mentionedUsers = append(mentionedUsers, asStr(u["user_id"]))
		}
	}
	doc := bson.M{
		"comment_id": newID("cmt"), "org_id": orgID, "task_id": taskID,
		"author_id": user["user_id"], "body": b.Body,
		"mentions": mentionedUsers, "mentions_emails": mentions,
		"created_at": isoNow(),
	}
	if err := insertDoc(ctx, colComments, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	doc["author_name"] = bsonStr(user, "name", bsonStr(user, "email", ""))
	doc["author_email"] = user["email"]
	doc["author_picture"] = user["picture"]
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
	tq := bson.M{"org_id": orgID}
	if !all {
		tq["project_id"] = bson.M{"$in": allowed}
	}
	tasks, err := findMany(ctx, colTasks, tq, bson.M{"_id": 0}, nil, 5000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	accessibleTaskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		accessibleTaskIDs = append(accessibleTaskIDs, asStr(t["task_id"]))
	}
	eq := bson.M{"org_id": orgID}
	if !all {
		eq["task_id"] = bson.M{"$in": accessibleTaskIDs}
	}
	entries, err := findMany(ctx, colEntries, eq, bson.M{"_id": 0}, nil, 5000)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}

	today := nowUTC()
	startQ, endQ := c.QueryParam("start"), c.QueryParam("end")
	periodMode := startQ != "" && endQ != ""
	var dateList []string
	var periodTasks, scopedEntries []bson.M
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
			periodTasks = []bson.M{}
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

	byStatus := bson.M{"todo": 0, "in_progress": 0, "review": 0, "done": 0}
	for _, t := range periodTasks {
		s := bsonStr(t, "status", "todo")
		byStatus[s] = asInt(byStatus[s]) + 1
	}

	completedSeries := make([]bson.M, 0, len(dateList))
	for _, d := range dateList {
		c := 0
		for _, t := range tasks {
			if strings.HasPrefix(asStr(t["completed_at"]), d) {
				c++
			}
		}
		completedSeries = append(completedSeries, bson.M{"date": d, "completed": c})
	}
	timeSeries := make([]bson.M, 0, len(dateList))
	for _, d := range dateList {
		mins := 0
		for _, e := range scopedEntries {
			if asStr(e["date"]) == d {
				mins += asInt(e["minutes"])
			}
		}
		timeSeries = append(timeSeries, bson.M{"date": d, "minutes": mins})
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
	return c.JSON(http.StatusOK, bson.M{
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
