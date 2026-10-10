package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
)

var jwtSecret []byte

// createAccessToken matches python's JWT payload exactly so existing tokens
// survive the migration.
func createAccessToken(userID, email string) string {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"type":  "access",
		"exp":   nowUTC().Add(7 * 24 * time.Hour).Unix(),
	}
	t, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	return t
}

// currentUser mirrors python's get_current_user: cookie first, then Bearer.
func currentUser(c echo.Context) (bson.M, *echo.HTTPError) {
	req := c.Request()
	token := ""
	if ck, err := req.Cookie("access_token"); err == nil {
		token = ck.Value
	}
	if token == "" {
		if auth := req.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = auth[7:]
		}
	}
	if token == "" {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Not authenticated")
	}
	tok, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) { return jwtSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, echo.NewHTTPError(http.StatusUnauthorized, "Token expired")
		}
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || !tok.Valid {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
	}
	sub, _ := claims["sub"].(string)
	user, err := findOne(req.Context(), colUsers, bson.M{"user_id": sub}, bson.M{"_id": 0, "password_hash": 0})
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if user == nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "User not found")
	}
	return user, nil
}

func setAuthCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name: "access_token", Value: token, Path: "/",
		MaxAge: 7 * 24 * 60 * 60, HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode,
	})
}

func clearAuthCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name: "access_token", Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode,
	})
}

func ensureMember(c echo.Context, orgID, userID string) (bson.M, *echo.HTTPError) {
	m, err := findOne(c.Request().Context(), colMembers, bson.M{"org_id": orgID, "user_id": userID}, bson.M{"_id": 0})
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if m == nil {
		return nil, echo.NewHTTPError(http.StatusForbidden, "Not a member of this organization")
	}
	return m, nil
}

func roleIs(m bson.M, roles ...string) bool {
	r := asStr(m["role"])
	for _, want := range roles {
		if r == want {
			return true
		}
	}
	return false
}
