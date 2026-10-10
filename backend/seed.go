package main

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func initIndexes(ctx context.Context) error {
	index := func(col *mongo.Collection, model mongo.IndexModel) error {
		_, err := col.Indexes().CreateOne(ctx, model)
		return err
	}
	unique := func(keys interface{}) mongo.IndexModel {
		return mongo.IndexModel{Keys: keys, Options: options.Index().SetUnique(true)}
	}
	steps := []struct {
		col   *mongo.Collection
		model mongo.IndexModel
	}{
		{colUsers, unique(bson.M{"email": 1})},
		{colUsers, unique(bson.M{"user_id": 1})},
		{colOrgs, unique(bson.M{"org_id": 1})},
		{colMembers, unique(bson.D{{Key: "org_id", Value: 1}, {Key: "user_id", Value: 1}})},
		{colProjects, unique(bson.M{"project_id": 1})},
		{colTasks, unique(bson.M{"task_id": 1})},
		{colTasks, mongo.IndexModel{Keys: bson.M{"org_id": 1}}},
		{colSprints, unique(bson.M{"sprint_id": 1})},
		{colEntries, unique(bson.M{"entry_id": 1})},
		{colInvites, unique(bson.M{"invite_id": 1})},
		// TTL: Mongo deletes expired invites
		{colInvites, mongo.IndexModel{Keys: bson.M{"expires_at": 1}, Options: options.Index().SetExpireAfterSeconds(0)}},
		{colRefresh, unique(bson.M{"token_hash": 1})},
		// TTL: Mongo deletes expired refresh tokens
		{colRefresh, mongo.IndexModel{Keys: bson.M{"expires_at": 1}, Options: options.Index().SetExpireAfterSeconds(0)}},
	}
	for _, s := range steps {
		if err := index(s.col, s.model); err != nil {
			return err
		}
	}
	return nil
}

func seed(ctx context.Context) {
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
		})
		_ = pgPoolInsert(ctx, "memberships", map[string]any{
			"membership_id": newID("mem"), "org_id": orgID,
			"user_id": userID, "role": "owner", "created_at": isoNow(),
		})
		// Seed a demo project + sprint + tasks
		prjID := newID("prj")
		_ = insertDoc(ctx, colProjects, bson.M{
			"project_id": prjID, "org_id": orgID,
			"name": "Web Redesign", "key": "WEB",
			"description": "Company website redesign project",
			"color":       "#4F46E5", "status": "active", "created_by": userID,
			"created_at": isoNow(),
		})
		_ = insertDoc(ctx, colProjMem, bson.M{
			"org_id": orgID, "project_id": prjID, "user_id": userID,
			"role": "lead", "added_at": isoNow(), "added_by": userID,
		})
		sprID := newID("spr")
		_ = insertDoc(ctx, colSprints, bson.M{
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
			_ = insertDoc(ctx, colTasks, bson.M{
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

// backfill: ensure every project has its creator as a project_member
// (idempotent), and default status. Runs regardless of SEED_DEMO.
func backfill(ctx context.Context) {
	allProjects, err := findMany(ctx, colProjects, bson.M{}, bson.M{"_id": 0}, nil, 5000)
	if err != nil {
		log.Fatalf("backfill failed: %v", err)
	}
	for _, p := range allProjects {
		if asStr(p["status"]) == "" {
			_, _ = colProjects.UpdateOne(ctx, bson.M{"project_id": p["project_id"]},
				bson.M{"$set": bson.M{"status": "active"}})
		}
		createdBy := asStr(p["created_by"])
		if createdBy == "" {
			continue
		}
		exists, err := findOne(ctx, colProjMem, bson.M{
			"org_id": p["org_id"], "project_id": p["project_id"], "user_id": createdBy,
		}, nil)
		if err != nil || exists != nil {
			continue
		}
		_ = insertDoc(ctx, colProjMem, bson.M{
			"org_id": p["org_id"], "project_id": p["project_id"], "user_id": createdBy,
			"role": "lead", "added_at": isoNow(), "added_by": createdBy,
		})
	}
}
