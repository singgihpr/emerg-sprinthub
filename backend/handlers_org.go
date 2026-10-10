package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/image/draw"
)

func listOrgs(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	memberships, err := pgFindMany(ctx, "memberships", map[string]any{"user_id": user["user_id"]}, nil, "created_at", 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgIDs := make([]string, 0, len(memberships))
	roleMap := map[string]any{}
	for _, m := range memberships {
		orgIDs = append(orgIDs, asStr(m["org_id"]))
		roleMap[asStr(m["org_id"])] = m["role"]
	}
	if len(orgIDs) == 0 {
		return c.JSON(http.StatusOK, []map[string]any{})
	}
	// Custom IN query because the minimal helper only does equality.
	placeholders := make([]string, len(orgIDs))
	args := make([]any, len(orgIDs))
	for i, id := range orgIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf("SELECT * FROM organizations WHERE org_id IN (%s) ORDER BY created_at",
		strings.Join(placeholders, ", "))
	rows, err := pgTxFromContext(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	res, err := rows.Query(ctx, q, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgs, err := scanRowsToMaps(res)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	for _, o := range orgs {
		o["role"] = roleMap[asStr(o["org_id"])]
	}
	return c.JSON(http.StatusOK, orgs)
}

func createOrg(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	var b orgCreate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Name == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	orgID := newID("org")
	doc := map[string]any{
		"org_id": orgID, "name": b.Name, "owner_id": user["user_id"],
		"logo": b.Logo, "created_at": isoNow(),
		"plan_id": "free", "subscription_status": "active",
	}
	if err := pgInsert(ctx, "organizations", doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if err := pgInsert(ctx, "org_quotas", map[string]any{
		"org_id": orgID, "current_members": 1, "current_projects": 0,
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to initialize quotas")
	}
	_ = pgInsert(ctx, "memberships", map[string]any{
		"membership_id": newID("mem"), "org_id": orgID,
		"user_id": user["user_id"], "role": "owner", "created_at": isoNow(),
	})
	ownerRoleID := newID("role")
	_ = pgInsert(ctx, "roles", map[string]any{
		"role_id": ownerRoleID, "org_id": orgID, "name": "Owner",
		"description": "Full access to everything",
		"permissions": []string{"manage_org", "manage_billing", "invite_members", "manage_roles", "view_analytics", "create_project", "edit_project", "delete_project", "manage_project_members", "create_task", "edit_task", "delete_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"},
		"is_default": false, "is_system": true, "created_at": isoNow(), "updated_at": isoNow(),
	})
	memberRoleID := newID("role")
	_ = pgInsert(ctx, "roles", map[string]any{
		"role_id": memberRoleID, "org_id": orgID, "name": "Member",
		"description": "Standard access",
		"permissions": []string{"create_task", "edit_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"},
		"is_default": true, "is_system": true, "created_at": isoNow(), "updated_at": isoNow(),
	})
	_ = pgInsert(ctx, "role_assignments", map[string]any{
		"assignment_id": newID("ra"), "org_id": orgID,
		"user_id": user["user_id"], "role_id": ownerRoleID, "created_at": isoNow(),
	})
	doc["role"] = "owner"
	return c.JSON(http.StatusOK, doc)
}

func updateOrg(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermManageOrg); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b orgUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	updates := map[string]any{}
	if b.Name != nil {
		updates["name"] = *b.Name
	}
	if b.Logo != nil {
		updates["logo"] = *b.Logo
	}
	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "No fields to update")
	}
	ctx := c.Request().Context()
	if err := pgUpdate(ctx, "organizations", map[string]any{"org_id": orgID}, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	org, err := pgFindOne(ctx, "organizations", map[string]any{"org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if org == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Workspace not found")
	}
	return c.JSON(http.StatusOK, org)
}

// ---- logo upload ----

// ponytail: stdlib image + x/image/draw, PNG output. Pillow produced WebP;
// Go stdlib cannot encode webp. <img src=data:...> renders both, so old logos
// keep working. Switch to a webp encoder only if payload size matters.
const maxLogoUpload = 5 * 1024 * 1024
const logoSize = 256

func resizeToDataURL(raw []byte) (string, error) {
	im, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	m := w
	if h < m {
		m = h
	}
	left := (w - m) / 2
	top := (h - m) / 2
	src := image.NewRGBA(image.Rect(0, 0, m, m))
	draw.CatmullRom.Scale(src, src.Bounds(), im, image.Rect(left, top, left+m, top+m), draw.Over, nil)
	out := image.NewRGBA(image.Rect(0, 0, logoSize, logoSize))
	draw.CatmullRom.Scale(out, out.Bounds(), src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func uploadOrgLogo(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermManageOrg); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	if !strings.HasPrefix(fh.Header.Get("Content-Type"), "image/") {
		return echo.NewHTTPError(http.StatusBadRequest, "File must be an image")
	}
	src, err := fh.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	defer src.Close()
	raw, err := io.ReadAll(src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if len(raw) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Empty file")
	}
	if len(raw) > maxLogoUpload {
		return echo.NewHTTPError(http.StatusBadRequest, "Image too large (max 5 MB)")
	}
	dataURL, err := resizeToDataURL(raw)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid image file")
	}
	ctx := c.Request().Context()
	if err := pgUpdate(ctx, "organizations", map[string]any{"org_id": orgID}, map[string]any{"logo": dataURL}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	org, err := pgFindOne(ctx, "organizations", map[string]any{"org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if org == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Workspace not found")
	}
	return c.JSON(http.StatusOK, org)
}

// ---- members ----

func listMembers(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if _, herr := ensureMember(c, orgID, asStr(user["user_id"])); herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	members, err := pgFindMany(ctx, "memberships", map[string]any{"org_id": orgID}, nil, "created_at", 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, asStr(m["user_id"]))
	}
	umap := map[string]map[string]any{}
	for _, uid := range userIDs {
		u, err := pgFindOne(ctx, "users", map[string]any{"user_id": uid}, nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		if u != nil {
			umap[uid] = u
		}
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		u := umap[asStr(m["user_id"])]
		if u == nil {
			u = map[string]any{}
		}
		ph := asStr(u["password_hash"])
		invited := ph == "" && asStr(u["auth_provider"]) != "google"
		delete(u, "password_hash")
		row := map[string]any{}
		for k, v := range u {
			row[k] = v
		}
		row["role"] = m["role"]
		row["membership_id"] = m["membership_id"]
		row["invited"] = invited
		out = append(out, row)
	}
	return c.JSON(http.StatusOK, out)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func urlsafeToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func inviteMember(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermInviteMembers); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b memberInvite
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if !vEmail(b.Email) || b.Name == "" || b.Role == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	email := strings.ToLower(b.Email)
	org, err := pgFindOne(ctx, "organizations", map[string]any{"org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgName := cfg.EmailFromName
	if org != nil && asStr(org["name"]) != "" {
		orgName = asStr(org["name"])
	}
	var targetID, name string
	needsPassword := true
	existing, err := pgFindOne(ctx, "users", map[string]any{"email": email}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if existing != nil {
		targetID = asStr(existing["user_id"])
		name = mapStrDefault(existing, "name", b.Name)
		needsPassword = asStr(existing["password_hash"]) == "" && asStr(existing["auth_provider"]) != "google"
	} else {
		targetID = newID("user")
		name = b.Name
		_ = pgInsert(ctx, "users", map[string]any{
			"user_id": targetID, "email": email, "name": b.Name,
			"password_hash": nil, "picture": nil, "created_at": isoNow(),
		})
	}
	already, err := pgFindOne(ctx, "memberships", map[string]any{"org_id": orgID, "user_id": targetID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if already != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "User already a member")
	}
	if herr := checkQuota(ctx, orgID, "members", 1); herr != nil {
		return herr
	}
	role := b.Role
	if role == "" {
		role = "member"
	}
	_ = pgInsert(ctx, "memberships", map[string]any{
		"membership_id": newID("mem"), "org_id": orgID,
		"user_id": targetID, "role": role, "created_at": isoNow(),
	})
	defaultRole, err := pgFindOne(ctx, "roles", map[string]any{"org_id": orgID, "is_default": true}, nil)
	if err == nil && defaultRole != nil {
		_ = pgInsert(ctx, "role_assignments", map[string]any{
			"assignment_id": newID("ra"), "org_id": orgID,
			"user_id": targetID, "role_id": asStr(defaultRole["role_id"]), "created_at": isoNow(),
		})
	}
	incrementQuota(ctx, orgID, "members", 1)
	logUsageEvent(ctx, orgID, "member_added", 1)
	token := ""
	if needsPassword {
		token = urlsafeToken()
		// Re-invite replaces the pending token for this org only; other orgs keep theirs.
		if err := pgDelete(ctx, "invites", map[string]any{"org_id": orgID, "email": email}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		_ = pgInsert(ctx, "invites", map[string]any{
			"invite_id": newID("inv"), "token_hash": hashToken(token),
			"org_id": orgID, "email": email, "name": name, "role": role,
			"invited_by": user["user_id"], "created_at": isoNow(),
			"expires_at": nowUTC().Add(7 * 24 * time.Hour),
		})
	}
	if token != "" && !strings.HasPrefix(cfg.AppBaseURL, "https://") {
		log.Printf("APP_BASE_URL must be https:// to email invite links (got %q); invite email skipped", cfg.AppBaseURL)
	} else {
		inviter := mapStrDefault(user, "name", mapStrDefault(user, "email", "An admin"))
		subject, htmlBody := inviteEmail(name, inviter, orgName, token)
		go func(to, subject, html string) { _, _ = sendEmail(to, subject, html) }(email, subject, htmlBody)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "user_id": targetID, "invited": needsPassword})
}

func getInvite(c echo.Context) error {
	token := c.Param("token")
	ctx := c.Request().Context()
	inv, err := pgFindOne(ctx, "invites", map[string]any{"token_hash": hashToken(token)}, []string{"invite_id", "org_id", "email", "name", "role", "expires_at"})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if inv == nil || expiryTime(inv["expires_at"]).Before(nowUTC()) {
		return echo.NewHTTPError(http.StatusNotFound, "Invalid or expired invite")
	}
	org, err := pgFindOne(ctx, "organizations", map[string]any{"org_id": inv["org_id"]}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgName := ""
	if org != nil {
		orgName = asStr(org["name"])
	}
	role := mapStrDefault(inv, "role", "member")
	return c.JSON(http.StatusOK, map[string]any{
		"email": inv["email"], "name": inv["name"],
		"org_name": orgName, "role": role,
	})
}

func acceptInvite(c echo.Context) error {
	var b inviteAccept
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if !vPassword(b.Password) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	token := c.Param("token")
	ctx := c.Request().Context()
	inv, err := pgFindOne(ctx, "invites", map[string]any{"token_hash": hashToken(token)}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if inv == nil || expiryTime(inv["expires_at"]).Before(nowUTC()) {
		return echo.NewHTTPError(http.StatusNotFound, "Invalid or expired invite")
	}
	if err := pgDelete(ctx, "invites", map[string]any{"invite_id": inv["invite_id"]}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if err := pgUpdate(ctx, "users", map[string]any{"email": inv["email"]},
		map[string]any{"password_hash": hashPassword(b.Password)}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	u, err := pgFindOne(ctx, "users", map[string]any{"email": inv["email"]}, []string{"user_id", "email", "name", "picture", "created_at"})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if u == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Invited account no longer exists")
	}
	jwtToken := createAccessToken(asStr(u["user_id"]), asStr(u["email"]))
	setAuthCookie(c, jwtToken)
	return c.JSON(http.StatusOK, map[string]any{
		"user_id": u["user_id"], "email": u["email"], "name": u["name"],
		"picture": u["picture"], "token": jwtToken,
	})
}
