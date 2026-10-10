package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	portal "github.com/stripe/stripe-go/v82/billingportal/session"
	"github.com/stripe/stripe-go/v82/webhook"
)

type checkoutRequest struct {
	PlanID string `json:"plan_id"`
}

// createCheckoutSession: Stripe Checkout session for plan upgrade.
// Requires Stripe credentials in env; returns 503 if missing.
func createCheckoutSession(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if orgID == "" {
		orgID = extractOrgIDFromPath(c.Request().URL.Path)
	}
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermManageBilling); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	var b checkoutRequest
	if err := bindBody(c, &b); err != nil {
		return err
	}
	if b.PlanID == "" || (b.PlanID != "pro" && b.PlanID != "enterprise") {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Invalid plan")
	}
	if cfg.StripeSecretKey == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Billing not configured")
	}
	ctx := c.Request().Context()
	org, err := pgFindOne(ctx, "organizations", map[string]any{"org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if org == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Organization not found")
	}
	plan, err := pgFindOne(ctx, "plans", map[string]any{"plan_id": b.PlanID}, nil)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Plan not found")
	}
	priceID := asStr(plan["stripe_price_id"])
	if priceID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Plan not available for purchase")
	}
	stripe.Key = cfg.StripeSecretKey
	customerID := asStr(org["stripe_customer_id"])
	if customerID == "" {
		customerID = ""
	}
	successURL := cfg.AppBaseURL + "/billing/success?org_id=" + orgID
	cancelURL := cfg.AppBaseURL + "/billing/cancel?org_id=" + orgID
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	}
	if customerID != "" {
		params.Customer = stripe.String(customerID)
	} else {
		params.CustomerEmail = stripe.String(asStr(user["email"]))
	}
	params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
		Metadata: map[string]string{"org_id": orgID},
	}
	params.Metadata = map[string]string{"org_id": orgID}
	sess, err := session.New(params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create checkout session")
	}
	return c.JSON(http.StatusOK, map[string]any{"url": sess.URL, "session_id": sess.ID})
}

// createPortalSession: Stripe Customer Portal for managing subscription/billing.
func createPortalSession(c echo.Context) error {
	user, herr := currentUser(c)
	if herr != nil {
		return herr
	}
	orgID := c.Param("org_id")
	if orgID == "" {
		orgID = extractOrgIDFromPath(c.Request().URL.Path)
	}
	userID := asStr(user["user_id"])
	if _, herr := ensureMember(c, orgID, userID); herr != nil {
		return herr
	}
	if ok, err := hasPermission(c.Request().Context(), orgID, userID, PermManageBilling); err != nil || !ok {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}
	if cfg.StripeSecretKey == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Billing not configured")
	}
	ctx := c.Request().Context()
	org, err := pgFindOne(ctx, "organizations", map[string]any{"org_id": orgID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
	}
	if org == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Organization not found")
	}
	customerID := asStr(org["stripe_customer_id"])
	if customerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No active subscription")
	}
	stripe.Key = cfg.StripeSecretKey
	params := &stripe.BillingPortalSessionParams{
		Customer:     stripe.String(customerID),
		ReturnURL:    stripe.String(cfg.AppBaseURL + "/settings/billing?org_id=" + orgID),
	}
	sess, err := portal.New(params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create portal session")
	}
	return c.JSON(http.StatusOK, map[string]any{"url": sess.URL})
}

// stripeWebhook: handles Stripe events with signature verification.
// Events: checkout.session.completed, customer.subscription.updated/deleted.
func stripeWebhook(c echo.Context) error {
	if cfg.StripeWebhookSecret == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Webhook not configured")
	}
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Failed to read body")
	}
	sig := c.Request().Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(body, sig, cfg.StripeWebhookSecret)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid signature")
	}
	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid event data")
		}
		handleCheckoutComplete(sess)
	case "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid event data")
		}
		handleSubscriptionUpdate(sub, event.Type == "customer.subscription.deleted")
	}
	return c.JSON(http.StatusOK, map[string]any{"received": true})
}

func handleCheckoutComplete(sess stripe.CheckoutSession) {
	orgID := sess.Metadata["org_id"]
	if orgID == "" {
		return
	}
	customerID := ""
	if sess.Customer != nil {
		customerID = sess.Customer.ID
	}
	subscriptionID := ""
	if sess.Subscription != nil {
		subscriptionID = sess.Subscription.ID
	}
	ctx := context.Background()
	_ = pgPoolUpdate(ctx, "organizations", map[string]any{"org_id": orgID}, map[string]any{
		"stripe_customer_id":      customerID,
		"stripe_subscription_id":  subscriptionID,
		"subscription_status":     "active",
	})
	if sess.Subscription != nil && sess.Subscription.Items != nil && len(sess.Subscription.Items.Data) > 0 {
		priceID := sess.Subscription.Items.Data[0].Price.ID
		plan, _ := pgPoolFindOne(ctx, "plans", map[string]any{"stripe_price_id": priceID})
		if plan != nil {
			_ = pgPoolUpdate(ctx, "organizations", map[string]any{"org_id": orgID}, map[string]any{
				"plan_id": asStr(plan["plan_id"]),
			})
		}
	}
}

func handleSubscriptionUpdate(sub stripe.Subscription, deleted bool) {
	orgID := ""
	if sub.Metadata != nil {
		orgID = sub.Metadata["org_id"]
	}
	if orgID == "" {
		return
	}
	ctx := context.Background()
	if deleted {
		_ = pgPoolUpdate(ctx, "organizations", map[string]any{"org_id": orgID}, map[string]any{
			"subscription_status": "canceled",
			"plan_id":             "free",
		})
		return
	}
	status := "active"
	if sub.Status == "past_due" {
		status = "past_due"
	} else if sub.Status == "canceled" || sub.Status == "unpaid" {
		status = "canceled"
	}
	_ = pgPoolUpdate(ctx, "organizations", map[string]any{"org_id": orgID}, map[string]any{
		"subscription_status": status,
	})
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		priceID := sub.Items.Data[0].Price.ID
		plan, _ := pgPoolFindOne(ctx, "plans", map[string]any{"stripe_price_id": priceID})
		if plan != nil {
			_ = pgPoolUpdate(ctx, "organizations", map[string]any{"org_id": orgID}, map[string]any{
				"plan_id": asStr(plan["plan_id"]),
			})
		}
	}
}
