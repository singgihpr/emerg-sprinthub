package main

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
)

func register(c echo.Context) error {
	var b registerBody
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Email == "" || b.Password == "" || b.Name == "" || !vEmail(b.Email) || !vPassword(b.Password) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	email := strings.ToLower(b.Email)
	existing, err := findOne(ctx, colUsers, bson.M{"email": email}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if existing != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Email already registered")
	}
	userID := newID("user")
	user := bson.M{
		"user_id": userID, "email": email, "name": b.Name,
		"password_hash": hashPassword(b.Password), "picture": nil,
		"created_at": isoNow(),
	}
	if err := insertDoc(ctx, colUsers, user); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	orgID := newID("org")
	_ = insertDoc(ctx, colOrgs, bson.M{
		"org_id": orgID, "name": b.Name + "'s Workspace",
		"owner_id": userID, "logo": nil, "created_at": isoNow(),
	})
	_ = insertDoc(ctx, colMembers, bson.M{
		"membership_id": newID("mem"), "org_id": orgID,
		"user_id": userID, "role": "owner", "created_at": isoNow(),
	})
	token := createAccessToken(userID, email)
	setAuthCookie(c, token)
	setRefreshCookie(c, issueRefreshToken(ctx, userID))
	return c.JSON(http.StatusOK, bson.M{"user_id": userID, "email": email, "name": b.Name, "token": token})
}

func login(c echo.Context) error {
	var b loginBody
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.Email == "" || b.Password == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	email := strings.ToLower(b.Email)
	user, err := findOne(ctx, colUsers, bson.M{"email": email}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if user == nil || asStr(user["password_hash"]) == "" || !verifyPassword(b.Password, asStr(user["password_hash"])) {
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid email or password")
	}
	token := createAccessToken(asStr(user["user_id"]), email)
	setAuthCookie(c, token)
	setRefreshCookie(c, issueRefreshToken(ctx, asStr(user["user_id"])))
	return c.JSON(http.StatusOK, bson.M{
		"user_id": user["user_id"], "email": email,
		"name": user["name"], "picture": user["picture"], "token": token,
	})
}

// refresh rotates the refresh cookie (consume + reissue) and returns a fresh
// access token. Body may carry {"refresh_token": "..."} for non-cookie clients.
func refresh(c echo.Context) error {
	raw := ""
	if ck, err := c.Request().Cookie("refresh_token"); err == nil {
		raw = ck.Value
	}
	if raw == "" {
		var b struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = c.Bind(&b)
		raw = b.RefreshToken
	}
	if raw == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "Missing refresh token")
	}
	ctx := c.Request().Context()
	userID := consumeRefreshToken(ctx, raw)
	if userID == "" {
		clearRefreshCookie(c)
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or expired refresh token")
	}
	user, err := findOne(ctx, colUsers, bson.M{"user_id": userID}, bson.M{"_id": 0, "password_hash": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found")
	}
	token := createAccessToken(userID, asStr(user["email"]))
	setAuthCookie(c, token)
	setRefreshCookie(c, issueRefreshToken(ctx, userID))
	return c.JSON(http.StatusOK, bson.M{
		"user_id": user["user_id"], "email": user["email"],
		"name": user["name"], "picture": user["picture"], "token": token,
	})
}

func logout(c echo.Context) error {
	if ck, err := c.Request().Cookie("refresh_token"); err == nil && ck.Value != "" {
		// consume = revoke this session's refresh token
		_ = consumeRefreshToken(c.Request().Context(), ck.Value)
	}
	clearAuthCookie(c)
	clearRefreshCookie(c)
	return c.JSON(http.StatusOK, bson.M{"ok": true})
}

func me(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	return c.JSON(http.StatusOK, user)
}

func updateProfile(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	var b profileUpdate
	if err := bindBody(c, &b); err != nil {
		return err
	}
	updates := bson.M{}
	if b.Name != nil {
		updates["name"] = *b.Name
	}
	if b.Picture != nil {
		updates["picture"] = *b.Picture
	}
	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "No fields to update")
	}
	ctx := c.Request().Context()
	if _, err := colUsers.UpdateOne(ctx, bson.M{"user_id": user["user_id"]}, bson.M{"$set": updates}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	updated, err := findOne(ctx, colUsers, bson.M{"user_id": user["user_id"]}, bson.M{"_id": 0, "password_hash": 0})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	return c.JSON(http.StatusOK, updated)
}

func changePassword(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	var b passwordChange
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if !vPassword(b.NewPassword) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	ctx := c.Request().Context()
	u, err := findOne(ctx, colUsers, bson.M{"user_id": user["user_id"]}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if u == nil {
		return echo.NewHTTPError(http.StatusNotFound, "User not found")
	}
	if ph := asStr(u["password_hash"]); ph != "" {
		if b.CurrentPassword == nil || !verifyPassword(*b.CurrentPassword, ph) {
			return echo.NewHTTPError(http.StatusBadRequest, "Current password is incorrect")
		}
	}
	if _, err := colUsers.UpdateOne(ctx, bson.M{"user_id": user["user_id"]},
		bson.M{"$set": bson.M{"password_hash": hashPassword(b.NewPassword)}}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	// revoke every session, then reissue for this one so the user stays logged in
	revokeAllRefreshTokens(ctx, asStr(user["user_id"]))
	token := createAccessToken(asStr(user["user_id"]), asStr(user["email"]))
	setAuthCookie(c, token)
	setRefreshCookie(c, issueRefreshToken(ctx, asStr(user["user_id"])))
	return c.JSON(http.StatusOK, bson.M{"ok": true, "token": token})
}
