package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

var jwtSecret []byte

// access tokens are short-lived; long-lived sessions ride the refresh cookie.
const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour
)

// createAccessToken matches python's JWT payload exactly so existing tokens
// survive the migration.
func createAccessToken(userID, email string) string {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"type":  "access",
		"exp":   nowUTC().Add(accessTokenTTL).Unix(),
	}
	t, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	return t
}

// ---- refresh tokens: opaque random secrets, stored hashed, rotated on use ----

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newRefreshSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func issueRefreshToken(ctx context.Context, userID string) string {
	raw := newRefreshSecret()
	_ = pgInsert(ctx, "refresh_tokens", map[string]any{
		"token_hash": hashRefreshToken(raw), "user_id": userID,
		"expires_at": nowUTC().Add(refreshTokenTTL), "created_at": nowUTC(),
	})
	return raw
}

// consumeRefreshToken deletes the token (rotation): a replayed token is gone
// after the first use. Returns the user id, "" if invalid.
func consumeRefreshToken(ctx context.Context, raw string) string {
	tok := hashRefreshToken(raw)
	doc, err := pgFindOne(ctx, "refresh_tokens", map[string]any{"token_hash": tok}, nil)
	if err != nil || doc == nil {
		return ""
	}
	if nowUTC().After(expiryTime(doc["expires_at"])) {
		return ""
	}
	_ = pgDelete(ctx, "refresh_tokens", map[string]any{"token_hash": tok})
	return asStr(doc["user_id"])
}

func revokeAllRefreshTokens(ctx context.Context, userID string) {
	_ = pgDelete(ctx, "refresh_tokens", map[string]any{"user_id": userID})
}

const refreshCookiePath = "/api/auth"

func setRefreshCookie(c echo.Context, raw string) {
	c.SetCookie(&http.Cookie{
		Name: "refresh_token", Value: raw, Path: refreshCookiePath,
		MaxAge: 30 * 24 * 60 * 60, HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode,
	})
}

func clearRefreshCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name: "refresh_token", Value: "", Path: refreshCookiePath,
		MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode,
	})
}

// currentUser mirrors python's get_current_user: cookie first, then Bearer.
func currentUser(c echo.Context) (map[string]any, *echo.HTTPError) {
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
	user, err := pgFindOne(req.Context(), "users", map[string]any{"user_id": sub}, []string{"user_id", "email", "name", "picture", "created_at"})
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
		MaxAge: int(accessTokenTTL.Seconds()), HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode,
	})
}

func clearAuthCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name: "access_token", Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode,
	})
}

func ensureMember(c echo.Context, orgID, userID string) (map[string]any, *echo.HTTPError) {
	m, err := pgFindOne(c.Request().Context(), "memberships", map[string]any{"org_id": orgID, "user_id": userID}, nil)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if m == nil {
		return nil, echo.NewHTTPError(http.StatusForbidden, "Not a member of this organization")
	}
	return m, nil
}

func roleIs(m map[string]any, roles ...string) bool {
	r := asStr(m["role"])
	for _, want := range roles {
		if r == want {
			return true
		}
	}
	return false
}
