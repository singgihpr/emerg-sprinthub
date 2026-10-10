package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

func oidcConfigured() bool {
	return cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != ""
}

func oidcLogin(c echo.Context) error {
	if !oidcConfigured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "OIDC not configured")
	}
	ctx := c.Request().Context()
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "OIDC provider unavailable")
	}
	state := randomHex(16)
	c.SetCookie(&http.Cookie{
		Name: "oidc_state", Value: state, Path: "/",
		MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	redirectURL := cfg.AppBaseURL + "/api/auth/oidc/callback"
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	return c.Redirect(http.StatusTemporaryRedirect, oauthCfg.AuthCodeURL(state))
}

func oidcCallback(c echo.Context) error {
	if !oidcConfigured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "OIDC not configured")
	}
	stateCookie, _ := c.Cookie("oidc_state")
	stateParam := c.QueryParam("state")
	if stateCookie == nil || stateCookie.Value == "" || stateParam == "" || stateCookie.Value != stateParam {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid state parameter")
	}
	c.SetCookie(&http.Cookie{
		Name: "oidc_state", Value: "", Path: "/", MaxAge: -1,
	})
	ctx := c.Request().Context()
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "OIDC provider unavailable")
	}
	redirectURL := cfg.AppBaseURL + "/api/auth/oidc/callback"
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	token, err := oauthCfg.Exchange(ctx, c.QueryParam("code"))
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Code exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "Missing ID token")
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid ID token")
	}
	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid claims")
	}
	claims.Email = strings.ToLower(claims.Email)
	user, err := pgFindOne(ctx, "users", map[string]any{"email": claims.Email}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if user == nil {
		userID := newID("user")
		user = map[string]any{
			"user_id": userID, "email": claims.Email, "name": claims.Name,
			"password_hash": nil, "picture": nil, "created_at": isoNow(),
			"auth_provider": "oidc", "auth_provider_id": claims.Sub,
		}
		if err := pgInsert(ctx, "users", user); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
		}
		orgID := newID("org")
		_ = pgInsert(ctx, "organizations", map[string]any{
			"org_id": orgID, "name": claims.Name + "'s Workspace",
			"owner_id": userID, "logo": nil, "created_at": isoNow(),
		})
		_ = pgInsert(ctx, "memberships", map[string]any{
			"membership_id": newID("mem"), "org_id": orgID,
			"user_id": userID, "role": "owner", "created_at": isoNow(),
		})
	} else {
		provider := asStr(user["auth_provider"])
		if provider != "" && provider != "oidc" && provider != "local" {
			return echo.NewHTTPError(http.StatusBadRequest, "Account already linked to another provider")
		}
		if provider == "" || provider == "local" {
			_ = pgUpdate(ctx, "users", map[string]any{"user_id": user["user_id"]},
				map[string]any{"auth_provider": "oidc", "auth_provider_id": claims.Sub})
		}
	}
	userID := asStr(user["user_id"])
	email := asStr(user["email"])
	accessToken := createAccessToken(userID, email)
	setAuthCookie(c, accessToken)
	setRefreshCookie(c, issueRefreshToken(ctx, userID))
	return c.Redirect(http.StatusTemporaryRedirect, cfg.AppBaseURL)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
