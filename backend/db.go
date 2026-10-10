package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// ---- id / time helpers (match python formats) ----

func newID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func nowUTC() time.Time { return time.Now().UTC() }

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

// ---- value coercion ----

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

// expiryTime converts various time representations to time.Time.
func expiryTime(v interface{}) time.Time {
	switch x := v.(type) {
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

// Ensure context is used (for background jobs)
var _ = context.Background
