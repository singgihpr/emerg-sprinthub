package main

import (
	"net/http"
	"regexp"

	"github.com/labstack/echo/v4"
)

// ---- request DTOs (pointer = optional, mirrors pydantic Optionals) ----

type registerBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type orgCreate struct {
	Name string  `json:"name"`
	Logo *string `json:"logo"`
}

type orgUpdate struct {
	Name *string `json:"name"`
	Logo *string `json:"logo"`
}

type profileUpdate struct {
	Name    *string `json:"name"`
	Picture *string `json:"picture"`
}

type passwordChange struct {
	CurrentPassword *string `json:"current_password"`
	NewPassword     string  `json:"new_password"`
}

type memberInvite struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type inviteAccept struct {
	Password string `json:"password"`
}

type projectCreate struct {
	Name        string  `json:"name"`
	Key         string  `json:"key"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	Status      *string `json:"status"`
}

type projectUpdate struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	Status      *string `json:"status"`
}

type projectMemberAdd struct {
	UserID string  `json:"user_id"`
	Role   *string `json:"role"`
}

type sprintCreate struct {
	ProjectID string  `json:"project_id"`
	Name      string  `json:"name"`
	Goal      *string `json:"goal"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

type sprintUpdate struct {
	Name      *string `json:"name"`
	Goal      *string `json:"goal"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
	Status    *string `json:"status"`
}

type taskCreate struct {
	ProjectID     string   `json:"project_id"`
	Title         string   `json:"title"`
	Description   *string  `json:"description"`
	Status        string   `json:"status"`
	Priority      string   `json:"priority"`
	Type          string   `json:"type"`
	AssigneeID    *string  `json:"assignee_id"`
	SprintID      *string  `json:"sprint_id"`
	StartDate     *string  `json:"start_date"`
	DueDate       *string  `json:"due_date"`
	EstimateHours *float64 `json:"estimate_hours"`
	Repeat        *string  `json:"repeat"`
}

type taskUpdate struct {
	Title         *string  `json:"title"`
	Description   *string  `json:"description"`
	Status        *string  `json:"status"`
	Priority      *string  `json:"priority"`
	AssigneeID    *string  `json:"assignee_id"`
	SprintID      *string  `json:"sprint_id"`
	StartDate     *string  `json:"start_date"`
	DueDate       *string  `json:"due_date"`
	EstimateHours *float64 `json:"estimate_hours"`
}

type timeEntryCreate struct {
	TaskID  string  `json:"task_id"`
	Minutes int     `json:"minutes"`
	Note    *string `json:"note"`
	Date    *string `json:"date"`
}

type commentCreate struct {
	Body string `json:"body"`
}

// ---- validation (pydantic parity: bad payload -> 422) ----

func bindBody(c echo.Context, v interface{}) *echo.HTTPError {
	if err := c.Bind(v); err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Validation error")
	}
	return nil
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func vEmail(s string) bool { return emailRe.MatchString(s) }

func vPassword(s string) bool { return len(s) >= 12 && len(s) <= 128 }

var projectStatuses = map[string]bool{"planning": true, "active": true, "on_hold": true, "archived": true}
var sprintStatuses = map[string]bool{"planned": true, "active": true, "completed": true}
var repeatModes = map[string]bool{"none": true, "daily": true, "weekly": true, "monthly": true}

func vStatus(s string, allowed map[string]bool) bool { return allowed[s] }
