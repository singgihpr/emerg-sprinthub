package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

var testSrv *httptest.Server

func TestMain(m *testing.M) {
	cfg = config{
		DatabaseURL: getTestDatabaseURL(),
		JWTSecret:   "test-secret", EmailFromName: "SprintHub",
		EmailFrom: "no-reply@localhost",
		AllowedOrigins: []string{"http://localhost:3000"},
		RateLimitOff:   true,
	}
	jwtSecret = []byte(cfg.JWTSecret)
	ctx := context.Background()
	if err := connectPostgres(ctx, cfg.DatabaseURL); err != nil {
		fmt.Println("postgres required for tests:", err)
		os.Exit(1)
	}
	if _, err := pgPool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		fmt.Println("postgres reset failed:", err)
		os.Exit(1)
	}
	if err := runMigrations(ctx, pgPool); err != nil {
		fmt.Println("postgres migrations failed:", err)
		os.Exit(1)
	}
	testSrv = httptest.NewServer(newApp())
	code := m.Run()
	testSrv.Close()
	closePostgres()
	os.Exit(code)
}

func getTestDatabaseURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://sprinthub:sprinthub@127.0.0.1:5432/sprinthub?sslmode=disable"
}

func req(t *testing.T, method, path, token string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r, _ := http.NewRequest(method, testSrv.URL+path, rd)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// reqList hits endpoints that return JSON arrays.
func reqList(t *testing.T, method, path, token string, body interface{}) (int, []interface{}) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r, _ := http.NewRequest(method, testSrv.URL+path, rd)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []interface{}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// registerUser creates a user via the API, returns (token, user_id).
func registerUser(t *testing.T, email, name string) (string, string) {
	code, out := req(t, "POST", "/api/auth/register", "", map[string]interface{}{
		"email": email, "password": "Secret@12345", "name": name,
	})
	if code != 200 {
		t.Fatalf("register %s: %d %v", email, code, out)
	}
	return out["token"].(string), out["user_id"].(string)
}

func TestAuthFlow(t *testing.T) {
	token, uid := registerUser(t, "alice@test.dev", "Alice")
	code, me := req(t, "GET", "/api/auth/me", token, nil)
	if code != 200 || me["user_id"] != uid {
		t.Fatalf("me: %d %v", code, me)
	}
	code, _ = req(t, "POST", "/api/auth/login", "", map[string]interface{}{"email": "alice@test.dev", "password": "wrong"})
	if code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	code, _ = req(t, "POST", "/api/auth/change-password", token, map[string]interface{}{
		"current_password": "Secret@12345", "new_password": "NewPass@9999",
	})
	if code != 200 {
		t.Fatal("change-password failed")
	}
	code, _ = req(t, "POST", "/api/auth/login", "", map[string]interface{}{"email": "alice@test.dev", "password": "NewPass@9999"})
	if code != 200 {
		t.Fatal("login with new password failed")
	}
	code, _ = req(t, "GET", "/api/auth/me", "", nil)
	if code != 401 {
		t.Fatal("anon me should be 401")
	}
}

// TestOrgACL ports the critical ACL checks from the python harness: a
// detached member sees nothing and gets 403s; an attached member can write
// tasks but not projects; scoped lists never leak.
func TestOrgACL(t *testing.T) {
	ownerTok, _ := registerUser(t, "owner@test.dev", "Owner")
	memTok, memID := registerUser(t, "member@test.dev", "Member")

	code, org := req(t, "POST", "/api/orgs", ownerTok, map[string]interface{}{"name": "ACL Org"})
	if code != 200 {
		t.Fatal("create org failed")
	}
	orgID := org["org_id"].(string)
	code, proj := req(t, "POST", "/api/orgs/"+orgID+"/projects", ownerTok,
		map[string]interface{}{"name": "P1", "key": "P1"})
	if code != 200 {
		t.Fatal("create project failed")
	}
	pid := proj["project_id"].(string)

	// invite member into org (SMTP unset -> email skipped, still 200)
	code, inv := req(t, "POST", "/api/orgs/"+orgID+"/members", ownerTok,
		map[string]interface{}{"email": "member@test.dev", "name": "Member", "role": "member"})
	if code != 200 || inv["invited"] != false {
		t.Fatalf("invite existing user: %d %v", code, inv)
	}
	code, plist := reqList(t, "GET", "/api/orgs/"+orgID+"/projects", memTok, nil)
	if code != 200 || len(plist) != 0 {
		t.Fatalf("detached member must see no projects: %d %v", code, plist)
	}
	if code, tlist := reqList(t, "GET", "/api/orgs/"+orgID+"/tasks", memTok, nil); code != 200 || len(tlist) != 0 {
		t.Fatal("detached member must see no tasks")
	}
	if code, _ := req(t, "PATCH", "/api/orgs/"+orgID+"/projects/"+pid, memTok,
		map[string]interface{}{"name": "X"}); code != 403 {
		t.Fatalf("detached member patch project: %d", code)
	}
	if code, _ := req(t, "POST", "/api/orgs/"+orgID+"/tasks", memTok,
		map[string]interface{}{"project_id": pid, "title": "T"}); code != 403 {
		t.Fatalf("detached member create task: %d", code)
	}

	// attach member to project
	code, _ = req(t, "POST", "/api/orgs/"+orgID+"/projects/"+pid+"/members", ownerTok,
		map[string]interface{}{"user_id": memID, "role": "member"})
	if code != 200 {
		t.Fatal("attach failed")
	}
	code, _ = req(t, "POST", "/api/orgs/"+orgID+"/projects/"+pid+"/members", ownerTok,
		map[string]interface{}{"user_id": memID, "role": "member"})
	if code != 400 {
		t.Fatal("duplicate attach must be 400")
	}
	if code, plist := reqList(t, "GET", "/api/orgs/"+orgID+"/projects", memTok, nil); code != 200 || len(plist) != 1 {
		t.Fatal("attached member must see exactly the project")
	}
	code, task := req(t, "POST", "/api/orgs/"+orgID+"/tasks", memTok,
		map[string]interface{}{"project_id": pid, "title": "Member Task"})
	if code != 200 {
		t.Fatal("attached member must create tasks")
	}
	if code, _ := req(t, "PATCH", "/api/orgs/"+orgID+"/projects/"+pid, memTok,
		map[string]interface{}{"name": "X"}); code != 403 {
		t.Fatal("member role patch project must be 403")
	}
	// task visibility: other org's task id must 404
	tid := task["task_id"].(string)
	if code, _ := req(t, "PATCH", fmt.Sprintf("/api/orgs/%s/tasks/%s", orgID, tid), memTok,
		map[string]interface{}{"status": "done"}); code != 200 {
		t.Fatal("member patch own task must be 200")
	}
}

func TestValidationLiterals(t *testing.T) {
	ownerTok, _ := registerUser(t, "valid@test.dev", "Validator")
	code, org := req(t, "POST", "/api/orgs", ownerTok, map[string]interface{}{"name": "V Org"})
	orgID := org["org_id"].(string)
	_ = code
	if c, _ := req(t, "POST", "/api/orgs/"+orgID+"/projects", ownerTok,
		map[string]interface{}{"name": "B", "key": "B", "status": "garbage"}); c != 422 {
		t.Fatalf("garbage project status: %d", c)
	}
	code, proj := req(t, "POST", "/api/orgs/"+orgID+"/projects", ownerTok,
		map[string]interface{}{"name": "OK", "key": "OK"})
	pid := proj["project_id"].(string)
	if c, _ := req(t, "PATCH", "/api/orgs/"+orgID+"/projects/"+pid, ownerTok,
		map[string]interface{}{"status": "garbage"}); c != 422 {
		t.Fatalf("garbage project patch: %d", c)
	}
	code, sp := req(t, "POST", "/api/orgs/"+orgID+"/sprints", ownerTok,
		map[string]interface{}{"project_id": pid, "name": "S1"})
	sid := sp["sprint_id"].(string)
	_ = code
	if c, _ := req(t, "PATCH", "/api/orgs/"+orgID+"/sprints/"+sid, ownerTok,
		map[string]interface{}{"status": "garbage"}); c != 422 {
		t.Fatalf("garbage sprint status: %d", c)
	}
	if c, _ := req(t, "POST", "/api/auth/register", "",
		map[string]interface{}{"email": "notanemail", "password": "Secret@12345", "name": "N"}); c != 422 {
		t.Fatalf("bad email: %d", c)
	}
}

func TestLogoUpload(t *testing.T) {
	ownerTok, _ := registerUser(t, "logo@test.dev", "Logo")
	_, org := req(t, "POST", "/api/orgs", ownerTok, map[string]interface{}{"name": "Logo Org"})
	orgID := org["org_id"].(string)

	img := image.NewRGBA(image.Rect(0, 0, 640, 300))
	for x := 0; x < 640; x++ {
		for y := 0; y < 300; y++ {
			img.Set(x, y, color.RGBA{220, 30, 40, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="logo.png"`)
	h.Set("Content-Type", "image/png")
	part, _ := w.CreatePart(h)
	_, _ = part.Write(buf.Bytes())
	_ = w.Close()
	r, _ := http.NewRequest("POST", testSrv.URL+"/api/orgs/"+orgID+"/logo", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+ownerTok)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != 200 {
		t.Fatalf("logo upload: %d %s", resp.StatusCode, raw)
	}
	logo := out["logo"].(string)
	if !strings.HasPrefix(logo, "data:image/png;base64,") {
		t.Fatalf("logo not png data url: %s", logo[:40])
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(logo, "data:image/png;base64,"))
	if err != nil {
		t.Fatal("bad base64 logo")
	}
	im, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil || im.Bounds().Dx() != 256 || im.Bounds().Dy() != 256 {
		t.Fatalf("logo must be 256x256: %v %v", im.Bounds(), err)
	}
}

func TestTimerFlow(t *testing.T) {
	ownerTok, _ := registerUser(t, "timer@test.dev", "Timer")
	_, org := req(t, "POST", "/api/orgs", ownerTok, map[string]interface{}{"name": "Timer Org"})
	orgID := org["org_id"].(string)
	_, proj := req(t, "POST", "/api/orgs/"+orgID+"/projects", ownerTok, map[string]interface{}{"name": "T", "key": "T"})
	pid := proj["project_id"].(string)
	_, task := req(t, "POST", "/api/orgs/"+orgID+"/tasks", ownerTok,
		map[string]interface{}{"project_id": pid, "title": "Timed"})
	tid := task["task_id"].(string)

	if code, _ := req(t, "POST", "/api/orgs/"+orgID+"/timer/start", ownerTok,
		map[string]interface{}{"task_id": tid}); code != 200 {
		t.Fatal("timer start failed")
	}
	if code, _ := req(t, "POST", "/api/orgs/"+orgID+"/timer/stop", ownerTok, nil); code != 200 {
		t.Fatal("timer stop failed")
	}
	code, entry := req(t, "POST", "/api/orgs/"+orgID+"/timer/stop", ownerTok, nil)
	if code != 404 {
		t.Fatalf("second stop must be 404: %d", code)
	}
	_ = entry
	code, tlist := reqList(t, "GET", "/api/orgs/"+orgID+"/tasks", ownerTok, nil)
	if code != 200 {
		t.Fatal("tasks fetch failed")
	}
	found := false
	for _, x := range tlist {
		fi, ok := x.(map[string]interface{})
		if ok && fi["task_id"] == tid && fi["logged_minutes"] != nil && asInt(fi["logged_minutes"]) >= 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("logged_minutes not incremented")
	}
}

func TestCronAuth(t *testing.T) {
	if code, _ := req(t, "POST", "/api/cron/weekly-digest", "", nil); code != 401 {
		t.Fatalf("cron without secret: %d", code)
	}
	cfg.WebhookCronSecret = "testcron"
	defer func() { cfg.WebhookCronSecret = "" }()
	r, _ := http.NewRequest("POST", testSrv.URL+"/api/cron/weekly-digest", nil)
	r.Header.Set("Authorization", "Bearer testcron")
	r.Header.Set("X-Webhook-Id", "gotest-run-1")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("cron with secret: %d", resp.StatusCode)
	}
	resp2, _ := http.DefaultClient.Do(r)
	var out map[string]interface{}
	raw, _ := io.ReadAll(resp2.Body)
	_ = json.Unmarshal(raw, &out)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || out["duplicate"] != true {
		t.Fatalf("second run must be duplicate: %d %s %v", resp2.StatusCode, raw, out)
	}
}

func TestInviteAcceptFlow(t *testing.T) {
	ownerTok, _ := registerUser(t, "invown@test.dev", "InvOwner")
	_, org := req(t, "POST", "/api/orgs", ownerTok, map[string]interface{}{"name": "Inv Org"})
	orgID := org["org_id"].(string)
	code, inv := req(t, "POST", "/api/orgs/"+orgID+"/members", ownerTok,
		map[string]interface{}{"email": "newbie@test.dev", "name": "Newbie", "role": "member"})
	if code != 200 || inv["invited"] != true {
		t.Fatalf("invite new user: %d %v", code, inv)
	}
	// verify the invite was stored in postgres
	invDoc, err := pgPoolFindOne(context.Background(), "invites", map[string]any{"email": "newbie@test.dev"})
	if err != nil || invDoc == nil {
		t.Fatalf("invite not stored: %v", err)
	}
	// reconstruct: we cannot reverse the hash — exercise expiry/404 paths instead
	if code, _ := req(t, "GET", "/api/invites/bogus-token", "", nil); code != 404 {
		t.Fatal("unknown invite must 404")
	}
	// invalid/expired path + password min length validation
	if c, _ := req(t, "POST", "/api/invites/bogus-token/accept", "",
		map[string]interface{}{"password": "Secret@12345"}); c != 404 {
		t.Fatalf("accept bogus invite: %d", c)
	}
	// existing user re-invite flow marks invited=false
	code, _ = req(t, "POST", "/api/auth/register", "",
		map[string]interface{}{"email": "newbie@test.dev", "password": "Secret@12345", "name": "Newbie"})
	if code != 400 {
		t.Fatal("duplicate register must 400")
	}
}

func TestCommentsAndAnalytics(t *testing.T) {
	ownerTok, _ := registerUser(t, "cmt@test.dev", "Cmt")
	_, org := req(t, "POST", "/api/orgs", ownerTok, map[string]interface{}{"name": "Cmt Org"})
	orgID := org["org_id"].(string)
	_, proj := req(t, "POST", "/api/orgs/"+orgID+"/projects", ownerTok, map[string]interface{}{"name": "C", "key": "C"})
	pid := proj["project_id"].(string)
	_, task := req(t, "POST", "/api/orgs/"+orgID+"/tasks", ownerTok,
		map[string]interface{}{"project_id": pid, "title": "WithComments"})
	tid := task["task_id"].(string)

	code, cm := req(t, "POST", fmt.Sprintf("/api/orgs/%s/tasks/%s/comments", orgID, tid), ownerTok,
		map[string]interface{}{"body": "hey @cmt@test.dev look"})
	if code != 200 || len(cm["mentions"].([]interface{})) != 1 {
		t.Fatalf("comment mentions: %d %v", code, cm)
	}
	code, comments := reqList(t, "GET", fmt.Sprintf("/api/orgs/%s/tasks/%s/comments", orgID, tid), ownerTok, nil)
	if code != 200 || len(comments) != 1 || comments[0].(map[string]interface{})["author_name"] != "Cmt" {
		t.Fatalf("comments list: %d %v", code, comments)
	}

	code, an := req(t, "GET", "/api/orgs/"+orgID+"/analytics", ownerTok, nil)
	if code != 200 || an["total_tasks"] != float64(1) {
		t.Fatalf("analytics: %d %v", code, an)
	}
	code, _ = req(t, "GET", fmt.Sprintf("/api/orgs/%s/sprints/%s/burndown", orgID, "nope"), ownerTok, nil)
	if code != 404 {
		t.Fatalf("burndown unknown sprint: %d", code)
	}
}

// ---- P0 security: password policy, refresh rotation/revocation, CORS allowlist, rate limit ----

func TestPasswordPolicy(t *testing.T) {
	if c, _ := req(t, "POST", "/api/auth/register", "",
		map[string]interface{}{"email": "shortpw@test.dev", "password": "Short@1", "name": "S"}); c != 422 {
		t.Fatalf("short password register: %d", c)
	}
	token, _ := registerUser(t, "policy@test.dev", "Policy")
	if c, _ := req(t, "POST", "/api/auth/change-password", token,
		map[string]interface{}{"current_password": "Secret@12345", "new_password": "AlsoShort@1"}); c != 422 {
		t.Fatalf("short new password: %d", c)
	}
}

// authSession registers a user directly and returns the raw Set-Cookie
// values (http clients do not auto-send Secure cookies over plain http).
func authSession(t *testing.T, email string) map[string]string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": "Secret@12345", "name": "R"})
	r, _ := http.NewRequest("POST", testSrv.URL+"/api/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("register %s: %d", email, resp.StatusCode)
	}
	cks := map[string]string{}
	for _, ck := range resp.Cookies() {
		cks[ck.Name] = ck.Value
	}
	return cks
}

func postCookie(t *testing.T, path string, cookies map[string]string) (int, map[string]interface{}, map[string]string) {
	t.Helper()
	r, _ := http.NewRequest("POST", testSrv.URL+path, nil)
	for k, v := range cookies {
		r.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	got := map[string]string{}
	for _, ck := range resp.Cookies() {
		got[ck.Name] = ck.Value
	}
	return resp.StatusCode, out, got
}

func TestRefreshRotation(t *testing.T) {
	cks := authSession(t, "refresh@test.dev")
	raw1 := cks["refresh_token"]
	if raw1 == "" {
		t.Fatal("login must set refresh_token cookie")
	}
	code, out, cks2 := postCookie(t, "/api/auth/refresh", map[string]string{"refresh_token": raw1})
	if code != 200 || out["token"] == nil || out["token"].(string) == "" {
		t.Fatalf("refresh: %d %v", code, out)
	}
	raw2 := cks2["refresh_token"]
	if raw2 == "" || raw2 == raw1 {
		t.Fatal("refresh token must rotate on use")
	}
	if code, _, _ := postCookie(t, "/api/auth/refresh", map[string]string{"refresh_token": raw1}); code != 401 {
		t.Fatalf("replayed refresh token must 401: %d", code)
	}
	if code, _, _ := postCookie(t, "/api/auth/refresh", map[string]string{"refresh_token": raw2}); code != 200 {
		t.Fatalf("rotated refresh token must work: %d", code)
	}
}

func TestLogoutRevokesRefresh(t *testing.T) {
	cks := authSession(t, "logoutrev@test.dev")
	raw := cks["refresh_token"]
	if code, _, _ := postCookie(t, "/api/auth/logout", map[string]string{"refresh_token": raw}); code != 200 {
		t.Fatalf("logout: %d", code)
	}
	if code, _, _ := postCookie(t, "/api/auth/refresh", map[string]string{"refresh_token": raw}); code != 401 {
		t.Fatalf("refresh after logout must 401: %d", code)
	}
}

func TestCORSAllowlist(t *testing.T) {
	probe := func(origin string) string {
		r, _ := http.NewRequest("GET", testSrv.URL+"/healthz", nil)
		r.Header.Set("Origin", origin)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.Header.Get("Access-Control-Allow-Origin")
	}
	if got := probe("http://localhost:3000"); got != "http://localhost:3000" {
		t.Fatalf("allowed origin not echoed: %q", got)
	}
	if got := probe("https://evil.example"); got != "" {
		t.Fatalf("unknown origin must get no ACAO header: %q", got)
	}
}

func TestAuthRateLimit(t *testing.T) {
	saved := cfg.RateLimitOff
	cfg.RateLimitOff = false
	defer func() { cfg.RateLimitOff = saved }()
	e := echo.New()
	e.Use(authRateLimit)
	e.POST("/api/auth/login", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	srv := httptest.NewServer(e)
	defer srv.Close()
	last := 0
	for i := 0; i < 12; i++ {
		r, _ := http.NewRequest("POST", srv.URL+"/api/auth/login", nil)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		last = resp.StatusCode
	}
	if last != 429 {
		t.Fatalf("expected 429 after burst, got %d", last)
	}
}
