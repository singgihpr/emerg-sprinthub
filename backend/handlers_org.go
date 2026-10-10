package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/image/draw"
)

func listOrgs(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	ctx := c.Request().Context()
	memberships, err := findMany(ctx, colMembers, bson.M{"user_id": user["user_id"]}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgIDs := make([]string, 0, len(memberships))
	roleMap := bson.M{}
	for _, m := range memberships {
		orgIDs = append(orgIDs, asStr(m["org_id"]))
		roleMap[asStr(m["org_id"])] = m["role"]
	}
	orgs, err := findMany(ctx, colOrgs, bson.M{"org_id": bson.M{"$in": orgIDs}}, bson.M{"_id": 0},
		bson.D{{Key: "created_at", Value: 1}}, 500)
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
	doc := bson.M{
		"org_id": orgID, "name": b.Name, "owner_id": user["user_id"],
		"logo": b.Logo, "created_at": isoNow(),
	}
	if err := insertDoc(ctx, colOrgs, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	_ = insertDoc(ctx, colMembers, bson.M{
		"membership_id": newID("mem"), "org_id": orgID,
		"user_id": user["user_id"], "role": "owner", "created_at": isoNow(),
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
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin") {
		return echo.NewHTTPError(http.StatusForbidden, "Only owner or admin can edit workspace")
	}
	var b orgUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	updates := bson.M{}
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
	res, err := colOrgs.UpdateOne(ctx, bson.M{"org_id": orgID}, bson.M{"$set": updates})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if res.MatchedCount == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "Workspace not found")
	}
	org, err := findOne(ctx, colOrgs, bson.M{"org_id": orgID}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	org["role"] = m["role"]
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
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin") {
		return echo.NewHTTPError(http.StatusForbidden, "Only owner or admin can upload workspace logo")
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
	res, err := colOrgs.UpdateOne(ctx, bson.M{"org_id": orgID}, bson.M{"$set": bson.M{"logo": dataURL}})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if res.MatchedCount == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "Workspace not found")
	}
	org, err := findOne(ctx, colOrgs, bson.M{"org_id": orgID}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	org["role"] = m["role"]
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
	members, err := findMany(ctx, colMembers, bson.M{"org_id": orgID}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, asStr(m["user_id"]))
	}
	users, err := findMany(ctx, colUsers, bson.M{"user_id": bson.M{"$in": userIDs}}, bson.M{"_id": 0}, nil, 500)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	umap := bson.M{}
	for _, u := range users {
		umap[asStr(u["user_id"])] = u
	}
	out := make([]bson.M, 0, len(members))
	for _, m := range members {
		u, _ := umap[asStr(m["user_id"])].(bson.M)
		if u == nil {
			u = bson.M{}
		}
		ph := asStr(u["password_hash"])
		invited := ph == "" && asStr(u["auth_provider"]) != "google"
		delete(u, "password_hash")
		row := bson.M{}
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
	m, herr := ensureMember(c, orgID, asStr(user["user_id"]))
	if herr != nil {
		return herr
	}
	if !roleIs(m, "owner", "admin") {
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
	org, err := findOne(ctx, colOrgs, bson.M{"org_id": orgID}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgName := cfg.EmailFromName
	if org != nil && asStr(org["name"]) != "" {
		orgName = asStr(org["name"])
	}
	var targetID, name string
	needsPassword := true
	existing, err := findOne(ctx, colUsers, bson.M{"email": email}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if existing != nil {
		targetID = asStr(existing["user_id"])
		name = bsonStr(existing, "name", b.Name)
		needsPassword = asStr(existing["password_hash"]) == "" && asStr(existing["auth_provider"]) != "google"
	} else {
		targetID = newID("user")
		name = b.Name
		_ = insertDoc(ctx, colUsers, bson.M{
			"user_id": targetID, "email": email, "name": b.Name,
			"password_hash": nil, "picture": nil, "created_at": isoNow(),
		})
	}
	already, err := findOne(ctx, colMembers, bson.M{"org_id": orgID, "user_id": targetID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if already != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "User already a member")
	}
	role := b.Role
	if role == "" {
		role = "member"
	}
	_ = insertDoc(ctx, colMembers, bson.M{
		"membership_id": newID("mem"), "org_id": orgID,
		"user_id": targetID, "role": role, "created_at": isoNow(),
	})
	token := ""
	if needsPassword {
		token = urlsafeToken()
		// Re-invite replaces the pending token for this org only; other orgs keep theirs.
		if _, err := colInvites.DeleteMany(ctx, bson.M{"org_id": orgID, "email": email}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		_ = insertDoc(ctx, colInvites, bson.M{
			"invite_id": newID("inv"), "token_hash": hashToken(token),
			"org_id": orgID, "email": email, "name": name, "role": role,
			"invited_by": user["user_id"], "created_at": isoNow(),
			"expires_at": nowUTC().Add(7 * 24 * time.Hour),
		})
	}
	if token != "" && !strings.HasPrefix(cfg.AppBaseURL, "https://") {
		log.Printf("APP_BASE_URL must be https:// to email invite links (got %q); invite email skipped", cfg.AppBaseURL)
	} else {
		inviter := bsonStr(user, "name", bsonStr(user, "email", "An admin"))
		subject, htmlBody := inviteEmail(name, inviter, orgName, token)
		go func(to, subject, html string) { _, _ = sendEmail(to, subject, html) }(email, subject, htmlBody)
	}
	return c.JSON(http.StatusOK, bson.M{"ok": true, "user_id": targetID, "invited": needsPassword})
}

func getInvite(c echo.Context) error {
	token := c.Param("token")
	ctx := c.Request().Context()
	inv, err := findOne(ctx, colInvites, bson.M{"token_hash": hashToken(token)}, bson.M{"_id": 0, "token_hash": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if inv == nil || expiryTime(inv["expires_at"]).Before(nowUTC()) {
		return echo.NewHTTPError(http.StatusNotFound, "Invalid or expired invite")
	}
	org, err := findOne(ctx, colOrgs, bson.M{"org_id": inv["org_id"]}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgName := ""
	if org != nil {
		orgName = asStr(org["name"])
	}
	role := bsonStr(inv, "role", "member")
	return c.JSON(http.StatusOK, bson.M{
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
	inv, err := findOne(ctx, colInvites, bson.M{"token_hash": hashToken(token)}, bson.M{"_id": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if inv == nil || expiryTime(inv["expires_at"]).Before(nowUTC()) {
		return echo.NewHTTPError(http.StatusNotFound, "Invalid or expired invite")
	}
	deleted, err := colInvites.DeleteOne(ctx, bson.M{"invite_id": inv["invite_id"]})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if deleted.DeletedCount == 0 { // already accepted concurrently — single use
		return echo.NewHTTPError(http.StatusNotFound, "Invalid or expired invite")
	}
	if _, err := colUsers.UpdateOne(ctx, bson.M{"email": inv["email"]},
		bson.M{"$set": bson.M{"password_hash": hashPassword(b.Password)}}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	u, err := findOne(ctx, colUsers, bson.M{"email": inv["email"]}, bson.M{"_id": 0, "password_hash": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if u == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Invited account no longer exists")
	}
	jwtToken := createAccessToken(asStr(u["user_id"]), asStr(u["email"]))
	setAuthCookie(c, jwtToken)
	return c.JSON(http.StatusOK, bson.M{
		"user_id": u["user_id"], "email": u["email"], "name": u["name"],
		"picture": u["picture"], "token": jwtToken,
	})
}
