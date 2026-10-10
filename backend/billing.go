package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// planLimits mirrors the JSONB limits column on plans.
type planLimits struct {
	Members         int `json:"members"`
	Projects        int `json:"projects"`
	StorageMB       int `json:"storage_mb"`
	APICallsPerMonth int `json:"api_calls_per_month"`
}

// getPlanLimits fetches and parses the plan limits for an org.
func getPlanLimits(ctx context.Context, orgID string) (planLimits, *echo.HTTPError) {
	org, err := pgFindOne(ctx, "organizations", map[string]any{"org_id": orgID}, nil)
	if err != nil {
		return planLimits{}, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if org == nil {
		return planLimits{}, echo.NewHTTPError(http.StatusNotFound, "Organization not found")
	}
	planID := asStr(org["plan_id"])
	if planID == "" {
		planID = "free"
	}
	plan, err := pgFindOne(ctx, "plans", map[string]any{"plan_id": planID}, nil)
	if err != nil {
		return planLimits{}, echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if plan == nil {
		return planLimits{}, echo.NewHTTPError(http.StatusNotFound, "Plan not found")
	}
	limitsJSON, ok := plan["limits"]
	if !ok {
		return planLimits{}, echo.NewHTTPError(http.StatusInternalServerError, "Plan limits missing")
	}
	var limits planLimits
	switch v := limitsJSON.(type) {
	case []byte:
		if err := json.Unmarshal(v, &limits); err != nil {
			return planLimits{}, echo.NewHTTPError(http.StatusInternalServerError, "Invalid plan limits")
		}
	case string:
		if err := json.Unmarshal([]byte(v), &limits); err != nil {
			return planLimits{}, echo.NewHTTPError(http.StatusInternalServerError, "Invalid plan limits")
		}
	case map[string]any:
		b, _ := json.Marshal(v)
		if err := json.Unmarshal(b, &limits); err != nil {
			return planLimits{}, echo.NewHTTPError(http.StatusInternalServerError, "Invalid plan limits")
		}
	default:
		return planLimits{}, echo.NewHTTPError(http.StatusInternalServerError, "Unexpected plan limits type")
	}
	return limits, nil
}

// checkQuota returns 402 if the org would exceed its plan limit for the given resource.
// Called before createProject, inviteMember, etc.
// ponytail: soft-cap — checks current count vs limit. Does not block existing data over limit
// (e.g., downgrade with 10 members on Free plan that allows 5 — existing members stay,
// but no new members can be added until count drops below 5).
func checkQuota(ctx context.Context, orgID, resource string, increment int) *echo.HTTPError {
	limits, herr := getPlanLimits(ctx, orgID)
	if herr != nil {
		return herr
	}
	quota, err := pgFindOne(ctx, "org_quotas", map[string]any{"org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if quota == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Quota data missing")
	}
	var current, limit int
	switch resource {
	case "members":
		current = asInt(quota["current_members"])
		limit = limits.Members
	case "projects":
		current = asInt(quota["current_projects"])
		limit = limits.Projects
	case "api_calls":
		current = asInt(quota["api_calls_this_month"])
		limit = limits.APICallsPerMonth
		if resetAt, ok := quota["api_calls_reset_at"].(time.Time); ok && time.Now().After(resetAt) {
			_ = pgUpdate(ctx, "org_quotas", map[string]any{"org_id": orgID}, map[string]any{
				"api_calls_this_month": 0,
				"api_calls_reset_at":   time.Now().AddDate(0, 1, 0),
			})
			current = 0
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Unknown quota resource")
	}
	if limit > 0 && current+increment > limit {
		return echo.NewHTTPError(http.StatusPaymentRequired,
			fmt.Sprintf("Quota exceeded: %s limit is %d (current: %d). Upgrade your plan.", resource, limit, current))
	}
	return nil
}

// incrementQuota adds to current usage count in org_quotas.
func incrementQuota(ctx context.Context, orgID, resource string, delta int) {
	col := ""
	switch resource {
	case "members":
		col = "current_members"
	case "projects":
		col = "current_projects"
	case "api_calls":
		col = "api_calls_this_month"
	default:
		return
	}
	_ = pgExec(ctx, fmt.Sprintf("UPDATE org_quotas SET %s = %s + $1, updated_at = now() WHERE org_id = $2",
		col, col), delta, orgID)
}

// decrementQuota subtracts from current usage count in org_quotas.
func decrementQuota(ctx context.Context, orgID, resource string, delta int) {
	incrementQuota(ctx, orgID, resource, -delta)
}

// logUsageEvent records a usage event for metering/overage billing.
func logUsageEvent(ctx context.Context, orgID, eventType string, quantity int64) {
	_ = pgInsert(ctx, "usage_events", map[string]any{
		"event_id":   newID("evt"),
		"org_id":     orgID,
		"event_type": eventType,
		"quantity":   quantity,
		"timestamp":  isoNow(),
	})
}

// usageMeteringMiddleware logs an api_call event for every authenticated request.
// Runs after auth middleware so org_id is available.
func usageMeteringMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := next(c)
		if err != nil {
			return err
		}
		orgID := extractOrgIDFromPath(c.Request().URL.Path)
		if orgID == "" {
			return nil
		}
		userID := extractUserIDFromRequest(c.Request())
		if userID == "" {
			return nil
		}
		ctx := c.Request().Context()
		incrementQuota(ctx, orgID, "api_calls", 1)
		logUsageEvent(ctx, orgID, "api_call", 1)
		return nil
	}
}
