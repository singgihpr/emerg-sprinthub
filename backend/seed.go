package main

import (
	"context"
	"log"
)

func seed(ctx context.Context) {
	seedPlans(ctx)
	if !cfg.SeedDemo {
		log.Println("SEED_DEMO=false: skipping admin/demo seed (backfill only)")
		backfill(ctx)
		return
	}
	adminEmail := cfg.AdminEmail
	adminPassword := cfg.AdminPassword
	if adminEmail == "" {
		adminEmail = "widiardhana@gmail.com"
	}
	if adminPassword == "" {
		adminPassword = "Admin@1234"
	}
	existing, err := pgPoolFindOne(ctx, "users", map[string]any{"email": adminEmail})
	if err != nil {
		log.Fatalf("seed lookup failed: %v", err)
	}
	if existing == nil {
		userID := newID("user")
		_ = pgPoolInsert(ctx, "users", map[string]any{
			"user_id": userID, "email": adminEmail, "name": "Admin",
			"password_hash": hashPassword(adminPassword), "picture": nil,
			"created_at": isoNow(),
		})
		orgID := newID("org")
		_ = pgPoolInsert(ctx, "organizations", map[string]any{
			"org_id": orgID, "name": "Acme Corp",
			"owner_id": userID, "logo": nil, "created_at": isoNow(),
			"plan_id": "free", "subscription_status": "active",
		})
		_ = pgPoolInsert(ctx, "org_quotas", map[string]any{
			"org_id": orgID, "current_members": 1, "current_projects": 1,
		})
		_ = pgPoolInsert(ctx, "memberships", map[string]any{
			"membership_id": newID("mem"), "org_id": orgID,
			"user_id": userID, "role": "owner", "created_at": isoNow(),
		})
		ownerRoleID := newID("role")
		_ = pgPoolInsert(ctx, "roles", map[string]any{
			"role_id": ownerRoleID, "org_id": orgID, "name": "Owner",
			"description": "Full access to everything",
			"permissions": []string{"manage_org", "manage_billing", "invite_members", "manage_roles", "view_analytics", "create_project", "edit_project", "delete_project", "manage_project_members", "create_task", "edit_task", "delete_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"},
			"is_default": false, "is_system": true, "created_at": isoNow(), "updated_at": isoNow(),
		})
		memberRoleID := newID("role")
		_ = pgPoolInsert(ctx, "roles", map[string]any{
			"role_id": memberRoleID, "org_id": orgID, "name": "Member",
			"description": "Standard access",
			"permissions": []string{"create_task", "edit_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"},
			"is_default": true, "is_system": true, "created_at": isoNow(), "updated_at": isoNow(),
		})
		_ = pgPoolInsert(ctx, "role_assignments", map[string]any{
			"assignment_id": newID("ra"), "org_id": orgID,
			"user_id": userID, "role_id": ownerRoleID, "created_at": isoNow(),
		})
		// Seed a demo project + sprint + tasks
		prjID := newID("prj")
		_ = pgPoolInsert(ctx, "projects", map[string]any{
			"project_id": prjID, "org_id": orgID,
			"name": "Web Redesign", "key": "WEB",
			"description": "Company website redesign project",
			"color":       "#4F46E5", "status": "active", "created_by": userID,
			"created_at": isoNow(),
		})
		_ = pgPoolInsert(ctx, "project_members", map[string]any{
			"project_member_id": newID("pmem"), "org_id": orgID, "project_id": prjID, "user_id": userID,
			"role": "lead", "added_at": isoNow(), "added_by": userID,
		})
		sprID := newID("spr")
		_ = pgPoolInsert(ctx, "sprints", map[string]any{
			"sprint_id": sprID, "org_id": orgID, "project_id": prjID,
			"name": "Sprint 1", "goal": "Launch new landing page",
			"start_date": todayUTC(),
			"end_date":   nowUTC().AddDate(0, 0, 14).Format("2006-01-02"),
			"status":     "active", "created_at": isoNow(),
		})
		seedTasks := []struct {
			title, status, prio string
			est                 float64
		}{
			{"Design homepage hero", "in_progress", "high", 8},
			{"Set up analytics", "todo", "medium", 3},
			{"Write copy for pricing page", "review", "medium", 4},
			{"Fix mobile nav bug", "done", "high", 2},
			{"Daily standup", "todo", "low", 0.5},
		}
		for i, st := range seedTasks {
			var completedAt interface{}
			if st.status == "done" {
				completedAt = isoNow()
			}
			logged := 0
			if st.status == "done" {
				logged = 60
			}
			_ = pgPoolInsert(ctx, "tasks", map[string]any{
				"task_id": newID("tsk"), "org_id": orgID, "project_id": prjID,
				"title": st.title, "description": "", "status": st.status,
				"priority": st.prio, "type": "task", "assignee_id": userID,
				"sprint_id":      sprID,
				"start_date":     todayUTC(),
				"due_date":       nowUTC().AddDate(0, 0, i+2).Format("2006-01-02"),
				"estimate_hours": st.est, "logged_minutes": logged,
				"created_by":   userID,
				"created_at":   isoNow(),
				"updated_at":   isoNow(),
				"completed_at": completedAt,
			})
		}
	} else if !verifyPassword(adminPassword, asStr(existing["password_hash"])) {
		// update password to match .env
		_ = pgPoolUpdate(ctx, "users", map[string]any{"email": adminEmail},
			map[string]any{"password_hash": hashPassword(adminPassword)})
	}
	backfill(ctx)
	log.Println("Startup complete")
}

// seedPlans: ensure Free/Pro/Enterprise plans exist (idempotent).
// Generous limits: Free=5 members/3 projects/500MB/10k API calls;
// Pro=25/50/50GB/100k; Enterprise=1000/1000/500GB/1M (effectively unlimited).
func seedPlans(ctx context.Context) {
	plans := []struct {
		id, name string
		limits   string
	}{
		{"free", "Free", `{"members":5,"projects":3,"storage_mb":500,"api_calls_per_month":10000}`},
		{"pro", "Pro", `{"members":25,"projects":50,"storage_mb":51200,"api_calls_per_month":100000}`},
		{"enterprise", "Enterprise", `{"members":1000,"projects":1000,"storage_mb":512000,"api_calls_per_month":1000000}`},
	}
	for _, p := range plans {
		exists, _ := pgPoolFindOne(ctx, "plans", map[string]any{"plan_id": p.id})
		if exists == nil {
			_ = pgPoolInsert(ctx, "plans", map[string]any{
				"plan_id": p.id, "name": p.name, "stripe_price_id": nil,
				"limits": p.limits,
			})
		}
	}
	orgs, _ := pgPoolFind(ctx, "organizations", map[string]any{})
	for _, o := range orgs {
		if asStr(o["plan_id"]) == "" {
			_ = pgPoolUpdate(ctx, "organizations", map[string]any{"org_id": asStr(o["org_id"])},
				map[string]any{"plan_id": "free", "subscription_status": "active"})
		}
		qRow, _ := pgPoolFindOne(ctx, "org_quotas", map[string]any{"org_id": asStr(o["org_id"])})
		if qRow == nil {
			var memberCount, projectCount int64
			_ = pgPool.QueryRow(ctx, "SELECT COUNT(*) FROM memberships WHERE org_id = $1", asStr(o["org_id"])).Scan(&memberCount)
			_ = pgPool.QueryRow(ctx, "SELECT COUNT(*) FROM projects WHERE org_id = $1", asStr(o["org_id"])).Scan(&projectCount)
			_ = pgPoolInsert(ctx, "org_quotas", map[string]any{
				"org_id":           asStr(o["org_id"]),
				"current_members":  memberCount,
				"current_projects": projectCount,
			})
		}
	}
}

// backfill: ensure every project has its creator as a project_member
// (idempotent), and default status. Runs regardless of SEED_DEMO.
func backfill(ctx context.Context) {
	allProjects, err := pgPoolFind(ctx, "projects", map[string]any{})
	if err != nil {
		log.Fatalf("backfill failed: %v", err)
	}
	for _, p := range allProjects {
		if asStr(p["status"]) == "" {
			_ = pgPoolUpdate(ctx, "projects", map[string]any{"project_id": p["project_id"]},
				map[string]any{"status": "active"})
		}
		createdBy := asStr(p["created_by"])
		if createdBy == "" {
			continue
		}
		exists, err := pgPoolFindOne(ctx, "project_members", map[string]any{
			"org_id": p["org_id"], "project_id": p["project_id"], "user_id": createdBy,
		})
		if err != nil || exists != nil {
			continue
		}
		_ = pgPoolInsert(ctx, "project_members", map[string]any{
			"project_member_id": newID("pmem"), "org_id": p["org_id"], "project_id": p["project_id"], "user_id": createdBy,
			"role": "lead", "added_at": isoNow(), "added_by": createdBy,
		})
	}
}
