package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	mongoClient  *mongo.Client
	db           *mongo.Database
	colUsers     *mongo.Collection
	colOrgs      *mongo.Collection
	colMembers   *mongo.Collection
	colProjects  *mongo.Collection
	colProjMem   *mongo.Collection
	colSprints   *mongo.Collection
	colTasks     *mongo.Collection
	colRecurring *mongo.Collection
	colEntries   *mongo.Collection
	colTimers    *mongo.Collection
	colComments  *mongo.Collection
	colInvites   *mongo.Collection
	colCronRuns  *mongo.Collection
	colRefresh   *mongo.Collection
)

func connectDB(ctx context.Context, uri, name string) error {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return err
	}
	mongoClient = client
	db = client.Database(name)
	colUsers = db.Collection("users")
	colOrgs = db.Collection("organizations")
	colMembers = db.Collection("memberships")
	colProjects = db.Collection("projects")
	colProjMem = db.Collection("project_members")
	colSprints = db.Collection("sprints")
	colTasks = db.Collection("tasks")
	colRecurring = db.Collection("recurring_tasks")
	colEntries = db.Collection("time_entries")
	colTimers = db.Collection("active_timers")
	colComments = db.Collection("comments")
	colInvites = db.Collection("invites")
	colCronRuns = db.Collection("cron_runs")
	colRefresh = db.Collection("refresh_tokens")
	return nil
}

// ---- query helpers (python-style dynamic documents) ----

func findOne(ctx context.Context, col *mongo.Collection, filter, proj bson.M) (bson.M, error) {
	opts := options.FindOne()
	if proj != nil {
		opts.SetProjection(proj)
	}
	var doc bson.M
	err := col.FindOne(ctx, filter, opts).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func findMany(ctx context.Context, col *mongo.Collection, filter, proj bson.M, sort bson.D, limit int64) ([]bson.M, error) {
	opts := options.Find()
	if proj != nil {
		opts.SetProjection(proj)
	}
	if sort != nil {
		opts.SetSort(sort)
	}
	if limit > 0 {
		opts.SetLimit(limit)
	}
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	if docs == nil {
		docs = []bson.M{}
	}
	return docs, nil
}

// insertDoc inserts doc; the driver may set "_id" on the map, so strip it to
// mirror python's "insert then pop _id" flow.
func insertDoc(ctx context.Context, col *mongo.Collection, doc bson.M) error {
	if _, err := col.InsertOne(ctx, doc); err != nil {
		log.Println("insert failed:", err)
		return err
	}
	delete(doc, "_id")
	return nil
}

// ---- id / time helpers (match python formats) ----

func newID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func nowUTC() time.Time { return time.Now().UTC() }

// mongoCtx is for work outside a request (cron jobs, background emails).
func mongoCtx() context.Context { return context.Background() }

// isoNow matches python datetime.now(timezone.utc).isoformat()
func isoNow() string { return nowUTC().Format("2006-01-02T15:04:05.000000-07:00") }

func todayUTC() string { return nowUTC().Format("2006-01-02") }

var isoLayouts = []string{
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02",
}

func parseISO(s string) (time.Time, bool) {
	for _, l := range isoLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

// ---- bson value coercion ----

func asStr(v interface{}) string { s, _ := v.(string); return s }

func asFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func asInt(v interface{}) int { return int(asFloat(v)) }

func asBool(v interface{}) bool { b, _ := v.(bool); return b }

func bsonStr(m bson.M, key, fallback string) string {
	if s, ok := m[key].(string); ok && s != "" {
		return s
	}
	return fallback
}

func expiryTime(v interface{}) time.Time {
	switch x := v.(type) {
	case primitive.DateTime:
		return x.Time()
	case time.Time:
		return x
	case string:
		if t, ok := parseISO(x); ok {
			return t
		}
	}
	return time.Time{}
}

// first10 mirrors python's (s or "")[:10]
func first10(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// ---- misc ----

// isIPHost replaces python's ipaddress.ip_address() try/except.
func isIPHost(host string) bool {
	_, err := netip.ParseAddr(host)
	return err == nil
}

var shorteners = []string{"bit.ly", "tinyurl.com", "t.co", "is.gd", "cutt.ly", "goo.gl", "rebrand.ly"}

func hostOk(host string) bool {
	if host == "" || strings.Contains(host, "xn--") || isIPHost(host) {
		return false
	}
	for _, s := range shorteners {
		if host == s || strings.HasSuffix(host, "."+s) {
			return false
		}
	}
	return true
}

func sameSite(shown, real string) bool {
	return shown == real || strings.HasSuffix(real, "."+shown) || strings.HasSuffix(shown, "."+real)
}

// localRunID mirrors python f"local-{now_utc().timestamp()}"
func localRunID() string {
	return "local-" + strconv.FormatFloat(float64(nowUTC().UnixNano())/1e9, 'f', 6, 64)
}
