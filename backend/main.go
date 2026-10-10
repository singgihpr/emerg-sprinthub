package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
)

type config struct {
	MongoURL, DBName, JWTSecret string
	AdminEmail, AdminPassword   string
	EmailFromName, EmailFrom    string
	SMTPHost                    string
	SMTPPort                    int
	SMTPUser, SMTPPass          string
	AppBaseURL                  string
	WebhookCronSecret           string
}

var cfg config

func loadConfig() {
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	cfg = config{
		MongoURL:          get("MONGO_URL", ""),
		DBName:            get("DB_NAME", ""),
		JWTSecret:         get("JWT_SECRET", ""),
		AdminEmail:        get("ADMIN_EMAIL", ""),
		AdminPassword:     get("ADMIN_PASSWORD", ""),
		EmailFromName:     get("EMAIL_FROM_NAME", "SprintHub"),
		EmailFrom:         get("EMAIL_FROM", get("SMTP_USER", "no-reply@localhost")),
		SMTPHost:          get("SMTP_HOST", ""),
		SMTPPort:          587,
		SMTPUser:          get("SMTP_USER", ""),
		SMTPPass:          get("SMTP_PASS", ""),
		AppBaseURL:        get("APP_BASE_URL", ""),
		WebhookCronSecret: get("WEBHOOK_CRON_SECRET", ""),
	}
	if cfg.MongoURL == "" || cfg.DBName == "" || cfg.JWTSecret == "" {
		log.Fatal("MONGO_URL, DB_NAME and JWT_SECRET must be set")
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.SMTPPort = p
		}
	}
	cfg.AppBaseURL = strings.TrimRight(cfg.AppBaseURL, "/")
}

func main() {
	// backend/.env for local dev; docker-compose injects env directly and
	// godotenv never overrides existing vars.
	_ = godotenv.Load()
	loadConfig()
	jwtSecret = []byte(cfg.JWTSecret)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := connectDB(ctx, cfg.MongoURL, cfg.DBName); err != nil {
		log.Fatalf("mongo connect failed: %v", err)
	}
	defer func() {
		dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dcancel()
		if err := mongoClient.Disconnect(dctx); err != nil {
			log.Println("mongo disconnect:", err)
		}
	}()

	if err := initIndexes(ctx); err != nil {
		log.Fatalf("index creation failed: %v", err)
	}
	seed(ctx)
	go startScheduler()

	e := newApp()
	log.Fatal(e.Start(":8000"))
}

// newApp builds the echo server; separate from main so tests can mount it.
func newApp() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httpErrorHandler
	e.Use(corsMiddleware)

	e.GET("/healthz", func(c echo.Context) error { return c.JSON(http.StatusOK, bson.M{"ok": true}) })

	api := e.Group("/api")
	api.POST("/auth/register", register)
	api.POST("/auth/login", login)
	api.POST("/auth/logout", logout)
	api.GET("/auth/me", me)
	api.PATCH("/auth/me", updateProfile)
	api.POST("/auth/change-password", changePassword)

	api.GET("/orgs", listOrgs)
	api.POST("/orgs", createOrg)
	api.PATCH("/orgs/:org_id", updateOrg)
	api.POST("/orgs/:org_id/logo", uploadOrgLogo)
	api.GET("/orgs/:org_id/members", listMembers)
	api.POST("/orgs/:org_id/members", inviteMember)

	api.GET("/invites/:token", getInvite)
	api.POST("/invites/:token/accept", acceptInvite)

	api.GET("/orgs/:org_id/projects", listProjects)
	api.POST("/orgs/:org_id/projects", createProject)
	api.PATCH("/orgs/:org_id/projects/:project_id", updateProject)
	api.DELETE("/orgs/:org_id/projects/:project_id", deleteProject)
	api.GET("/orgs/:org_id/projects/:project_id/members", listProjectMembers)
	api.POST("/orgs/:org_id/projects/:project_id/members", addProjectMember)
	api.DELETE("/orgs/:org_id/projects/:project_id/members/:user_id", removeProjectMember)

	api.GET("/orgs/:org_id/sprints", listSprints)
	api.POST("/orgs/:org_id/sprints", createSprint)
	api.PATCH("/orgs/:org_id/sprints/:sprint_id", updateSprint)
	api.DELETE("/orgs/:org_id/sprints/:sprint_id", deleteSprint)
	api.GET("/orgs/:org_id/sprints/:sprint_id/burndown", sprintBurndown)

	api.GET("/orgs/:org_id/tasks", listTasks)
	api.POST("/orgs/:org_id/tasks", createTask)
	api.GET("/orgs/:org_id/recurring", listRecurring)
	api.DELETE("/orgs/:org_id/recurring/:recurring_id", stopRecurring)
	api.PATCH("/orgs/:org_id/tasks/:task_id", updateTask)
	api.DELETE("/orgs/:org_id/tasks/:task_id", deleteTask)

	api.GET("/orgs/:org_id/time-entries", listTimeEntries)
	api.POST("/orgs/:org_id/time-entries", createTimeEntry)
	api.GET("/orgs/:org_id/timer", getTimer)
	api.POST("/orgs/:org_id/timer/start", startTimer)
	api.POST("/orgs/:org_id/timer/stop", stopTimer)

	api.GET("/orgs/:org_id/team-activity", teamActivity)
	api.GET("/orgs/:org_id/tasks/:task_id/comments", listComments)
	api.POST("/orgs/:org_id/tasks/:task_id/comments", createComment)
	api.GET("/orgs/:org_id/analytics", analytics)

	api.POST("/cron/weekly-digest", cronWeeklyDigest)
	api.POST("/cron/spawn-recurring", cronSpawnRecurring)

	return e
}

// httpErrorHandler renders {"detail": ...} like the old FastAPI API did.
func httpErrorHandler(err error, c echo.Context) {
	code := http.StatusInternalServerError
	detail := "Internal server error"
	var he *echo.HTTPError
	if ok := asHTTPError(err, &he); ok {
		code = he.Code
		switch m := he.Message.(type) {
		case string:
			detail = m
		case error:
			detail = m.Error()
		default:
			detail = fmt.Sprint(m)
		}
	}
	if code >= http.StatusInternalServerError {
		log.Println("request error:", err)
	}
	if !c.Response().Committed {
		_ = c.JSON(code, bson.M{"detail": detail})
	}
}

func asHTTPError(err error, out **echo.HTTPError) bool {
	if he, ok := err.(*echo.HTTPError); ok {
		*out = he
		return true
	}
	return false
}

// corsMiddleware mirrors starlette CORSMiddleware with allow_origins=["*"],
// allow_credentials=True: echo the request origin instead of sending "*"
// (browsers reject credentialed wildcard responses).
func corsMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		origin := c.Request().Header.Get("Origin")
		if origin != "" {
			h := c.Response().Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Expose-Headers", "*")
			if c.Request().Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "DELETE, GET, HEAD, OPTIONS, PATCH, POST, PUT")
				h.Set("Access-Control-Allow-Headers", "*")
				return c.NoContent(http.StatusNoContent)
			}
		}
		return next(c)
	}
}
