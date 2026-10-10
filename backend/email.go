package main

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/net/html"
)

// ---- content scan (port of python _assert_safe_email) ----

var credAsk = []string{
	"reply with your password", "reply with the code", "send your password",
	"cvv", "send us your password", "enter your password below",
	"confirm your card number", "your full card number", "seed phrase",
	"recovery phrase", "verify your card", "social security number",
	"confirm your bank details",
}

var hostishRe = regexp.MustCompile(`(?i)\b(?:https?://)?((?:[a-z0-9-]+\.)+[a-z]{2,})`)

type emailAnchor struct{ href, text string }

func scanEmailHTML(body string) (tags map[string]bool, urls []string, anchors []emailAnchor) {
	tags = map[string]bool{}
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return
	}
	textOf := func(n *html.Node) string {
		var sb strings.Builder
		var walk func(*html.Node)
		walk = func(x *html.Node) {
			if x.Type == html.TextNode {
				sb.WriteString(x.Data)
			}
			for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
				walk(ch)
			}
		}
		walk(n)
		return sb.String()
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tags[strings.ToLower(n.Data)] = true
			var href string
			hasHref := false
			for _, a := range n.Attr {
				k := strings.ToLower(a.Key)
				if k == "href" || k == "src" {
					if a.Val != "" {
						urls = append(urls, a.Val)
					}
				}
				if k == "href" {
					href, hasHref = a.Val, true
				}
			}
			if strings.ToLower(n.Data) == "a" && hasHref {
				anchors = append(anchors, emailAnchor{href, textOf(n)})
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return
}

func assertSafeEmail(subject, htmlBody string) error {
	tags, urls, anchors := scanEmailHTML(htmlBody)
	for _, bad := range []string{"form", "input", "textarea", "select"} {
		if tags[bad] {
			return fmt.Errorf("No forms or input fields in email (G2)")
		}
	}
	body := strings.ToLower(subject + "\n" + htmlBody)
	for _, p := range credAsk {
		if strings.Contains(body, p) {
			return fmt.Errorf("Email asks the recipient for credentials: %q (G2)", p)
		}
	}
	for _, u := range urls {
		low := strings.ToLower(strings.TrimSpace(u))
		if strings.HasPrefix(low, "mailto:") || strings.HasPrefix(low, "tel:") ||
			strings.HasPrefix(low, "cid:") || strings.HasPrefix(low, "#") {
			continue
		}
		if !strings.HasPrefix(low, "https://") {
			return fmt.Errorf("Email links/assets must be absolute https: %q (G3)", u)
		}
		parsed, err := url.Parse(low)
		if err != nil {
			return fmt.Errorf("Invalid URL: %q (G3)", u)
		}
		host := parsed.Hostname()
		if !hostOk(host) || parsed.User != nil {
			return fmt.Errorf("Shortened, numeric-host or credential-bearing URL: %q (G3)", u)
		}
	}
	for _, a := range anchors {
		parsed, err := url.Parse(strings.ToLower(strings.TrimSpace(a.href)))
		if err != nil {
			continue
		}
		realHost := parsed.Hostname()
		if realHost == "" {
			continue
		}
		for _, m := range hostishRe.FindAllStringSubmatch(a.text, -1) {
			if !sameSite(strings.ToLower(m[1]), realHost) {
				return fmt.Errorf("Anchor text %q != real link host %q (G3)", m[1], realHost)
			}
		}
	}
	return nil
}

// ---- SMTP ----

func newMessageID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	domain := cfg.EmailFrom
	if i := strings.LastIndex(domain, "@"); i >= 0 {
		domain = domain[i+1:]
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hex.EncodeToString(b), domain)
}

func sendEmail(to, subject, htmlBody string) (string, error) {
	if err := assertSafeEmail(subject, htmlBody); err != nil {
		log.Println("send_email blocked by content scan:", err)
		return "", nil
	}
	if cfg.SMTPHost == "" {
		log.Println("No SMTP_HOST configured; skipping email send")
		return "", nil
	}
	msgID := newMessageID()
	msg, err := buildMIME(to, subject, htmlBody, msgID)
	if err != nil {
		log.Println("SMTP build failed:", err)
		return "", nil
	}
	if err := smtpSend(to, msg); err != nil {
		log.Println("SMTP send failed:", err)
		return "", nil
	}
	return msgID, nil
}

// buildMIME mirrors python EmailMessage with a text + html alternative pair.
func buildMIME(to, subject, htmlBody, msgID string) ([]byte, error) {
	boundary := "bnd-" + newID("m")
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.SetBoundary(boundary); err != nil {
		return nil, err
	}
	hdr := textproto.MIMEHeader{}
	hdr.Set("From", fmt.Sprintf("%s <%s>", cfg.EmailFromName, cfg.EmailFrom))
	hdr.Set("To", to)
	hdr.Set("Subject", subject)
	hdr.Set("Message-ID", msgID)
	hdr.Set("MIME-Version", "1.0")
	hdr.Set("Content-Type", "multipart/alternative; boundary=\""+boundary+"\"")
	for k, v := range hdr {
		fmt.Fprintf(&buf, "%s: %s\r\n", k, v[0])
	}
	buf.WriteString("\r\n")

	part, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return nil, err
	}
	qw := quotedprintable.NewWriter(part)
	fmt.Fprintf(qw, "%s\r\n\r\nThis email requires an HTML-capable client.\r\n", subject)
	qw.Close()

	htmlPart, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/html; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return nil, err
	}
	hw := quotedprintable.NewWriter(htmlPart)
	if _, err := hw.Write([]byte(htmlBody)); err != nil {
		return nil, err
	}
	if err := hw.Close(); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// smtpSend uses one blocking connection per send — fine at invite/digest
// volume. Swap to a queue/aiosmtplib equivalent if throughput matters.
// ponytail: connection-per-send; add a worker pool if throughput matters.
func smtpSend(to string, msg []byte) error {
	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort))
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	if cfg.SMTPUser != "" && cfg.SMTPPass != "" {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.SMTPHost}); err != nil {
			return err
		}
		auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(cfg.EmailFrom); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// ---- invite email template ----

func escHTML(s string) string { return html.EscapeString(s) }

func inviteEmail(name, inviterName, orgName, token string) (string, string) {
	if name == "" {
		name = "there"
	}
	subject := fmt.Sprintf("%s added you to %s on %s", inviterName, orgName, cfg.EmailFromName)
	link := cfg.AppBaseURL
	if token != "" {
		link = cfg.AppBaseURL + "/accept-invite/" + token
	}
	// assertSafeEmail requires absolute https links — omit the button otherwise
	button := ""
	if strings.HasPrefix(link, "https://") {
		label := "Open " + escHTML(cfg.EmailFromName)
		if token != "" {
			label = "Set password &amp; join"
		}
		button = fmt.Sprintf(`<p style="text-align:center;margin:28px 0"><a href="%s" `+
			`style="background:#4F46E5;color:#ffffff;text-decoration:none;padding:12px 28px;`+
			`border-radius:8px;font-size:15px;font-weight:600;display:inline-block">%s</a></p>`,
			escHTML(link), label)
	}
	var bodyLine string
	if token != "" {
		bodyLine = fmt.Sprintf(`<p style="color:#0F172A;font-size:15px">Hi %s, `+
			`<strong>%s</strong> invited you to <strong>%s</strong>. `+
			`Set your password to get started — the link expires in 7 days.</p>`,
			escHTML(name), escHTML(inviterName), escHTML(orgName))
	} else {
		bodyLine = fmt.Sprintf(`<p style="color:#0F172A;font-size:15px">Hi %s, `+
			`<strong>%s</strong> added you to <strong>%s</strong>.</p>`,
			escHTML(name), escHTML(inviterName), escHTML(orgName))
	}
	htmlBody := fmt.Sprintf(`
<table role="presentation" width="100%%" style="background:#F8FAFC;padding:32px 0;font-family:Arial,sans-serif">
  <tr><td align="center">
    <table role="presentation" width="600" style="background:#fff;border-radius:12px;overflow:hidden;box-shadow:0 8px 30px rgba(15,23,42,0.06)">
      <tr><td style="padding:28px 32px;background:#4F46E5;color:#fff">
        <div style="font-size:12px;letter-spacing:0.2em;text-transform:uppercase;opacity:0.8">%s</div>
        <div style="font-size:24px;font-weight:600;margin-top:6px">%s</div>
      </td></tr>
      <tr><td style="padding:24px 32px">
        %s
        %s
        <p style="font-size:12px;color:#94A3B8;margin-top:28px">Sent by %s. We never ask for your password by email.</p>
      </td></tr>
    </table>
  </td></tr>
</table>
`, escHTML(cfg.EmailFromName), escHTML(orgName), bodyLine, button, escHTML(cfg.EmailFromName))
	return subject, htmlBody
}

// ---- weekly digest ----

func buildAndSendDigest() {
	ctx := mongoCtx()
	today := nowUTC()
	weekStart := today.AddDate(0, 0, -7).Format("2006-01-02")
	memberships, err := findMany(ctx, colMembers,
		bson.M{"role": bson.M{"$in": []string{"owner", "admin"}}}, bson.M{"_id": 0}, nil, 2000)
	if err != nil {
		log.Println("digest membership fetch failed:", err)
		return
	}
	for _, m := range memberships {
		org, err := findOne(ctx, colOrgs, bson.M{"org_id": m["org_id"]}, bson.M{"_id": 0})
		if err != nil {
			continue
		}
		u, err := findOne(ctx, colUsers, bson.M{"user_id": m["user_id"]}, bson.M{"_id": 0, "password_hash": 0})
		if err != nil || u == nil || asStr(u["email"]) == "" || org == nil {
			continue
		}
		entries, err := findMany(ctx, colEntries,
			bson.M{"org_id": m["org_id"], "date": bson.M{"$gte": weekStart}}, bson.M{"_id": 0}, nil, 5000)
		if err != nil {
			continue
		}
		byUser := map[string]int{}
		for _, e := range entries {
			byUser[asStr(e["user_id"])] += asInt(e["minutes"])
		}
		userIDs := make([]string, 0, len(byUser))
		for uid := range byUser {
			userIDs = append(userIDs, uid)
		}
		umap := bson.M{}
		if len(userIDs) > 0 {
			users, err := findMany(ctx, colUsers, bson.M{"user_id": bson.M{"$in": userIDs}}, bson.M{"_id": 0, "password_hash": 0}, nil, 500)
			if err == nil {
				for _, x := range users {
					umap[asStr(x["user_id"])] = x
				}
			}
		}
		overdue, err := findMany(ctx, colTasks, bson.M{
			"org_id":   m["org_id"],
			"status":   bson.M{"$ne": "done"},
			"due_date": bson.M{"$lt": today.Format("2006-01-02")},
		}, bson.M{"_id": 0}, nil, 1000)
		if err != nil {
			continue
		}

		type row struct {
			uid  string
			mins int
		}
		rowsSorted := make([]row, 0, len(byUser))
		for uid, mins := range byUser {
			rowsSorted = append(rowsSorted, row{uid, mins})
		}
		sort.Slice(rowsSorted, func(i, j int) bool { return rowsSorted[i].mins > rowsSorted[j].mins })

		rowsHTML := ""
		if len(rowsSorted) == 0 {
			rowsHTML = `<tr><td colspan="2" style="padding:12px;color:#94A3B8">No time logged this week.</td></tr>`
		} else {
			var sb strings.Builder
			for _, r := range rowsSorted {
				uname := "Unknown"
				if ux, ok := umap[r.uid].(bson.M); ok {
					uname = bsonStr(ux, "name", bsonStr(ux, "email", "Unknown"))
				}
				fmt.Fprintf(&sb,
					`<tr><td style="padding:6px 12px;border-top:1px solid #E2E8F0">%s</td>`+
						`<td style="padding:6px 12px;border-top:1px solid #E2E8F0;text-align:right;font-family:monospace">%dh %dm</td></tr>`,
					escHTML(uname), r.mins/60, r.mins%60)
			}
			rowsHTML = sb.String()
		}

		overdueHTML := ""
		if len(overdue) == 0 {
			overdueHTML = `<li style="color:#94A3B8">No overdue tasks. Nice work.</li>`
		} else {
			var sb strings.Builder
			for i, t := range overdue {
				if i >= 10 {
					break
				}
				fmt.Fprintf(&sb, `<li style="padding:4px 0"><strong>%s</strong> `+
					`<span style="color:#94A3B8">— due %s</span></li>`,
					escHTML(asStr(t["title"])), escHTML(bsonStr(t, "due_date", "")))
			}
			overdueHTML = sb.String()
		}

		orgName := asStr(org["name"])
		subject := fmt.Sprintf("Weekly digest — %s", orgName)
		htmlBody := fmt.Sprintf(`
<table role="presentation" width="100%%" style="background:#F8FAFC;padding:32px 0;font-family:Arial,sans-serif">
  <tr><td align="center">
    <table role="presentation" width="600" style="background:#fff;border-radius:12px;overflow:hidden;box-shadow:0 8px 30px rgba(15,23,42,0.06)">
      <tr><td style="padding:28px 32px;background:#4F46E5;color:#fff">
        <div style="font-size:12px;letter-spacing:0.2em;text-transform:uppercase;opacity:0.8">Weekly Digest</div>
        <div style="font-size:24px;font-weight:600;margin-top:6px">%s</div>
      </td></tr>
      <tr><td style="padding:24px 32px">
        <p style="color:#0F172A;font-size:15px">Hi %s, here is your team snapshot for the last 7 days.</p>
        <h3 style="color:#0F172A;font-size:16px;margin-top:24px;margin-bottom:8px">Hours logged</h3>
        <table role="presentation" width="100%%" style="border-collapse:collapse;font-size:14px;color:#334155">%s</table>
        <h3 style="color:#0F172A;font-size:16px;margin-top:24px;margin-bottom:8px">Overdue tasks (%d)</h3>
        <ul style="color:#334155;font-size:14px;padding-left:20px;margin:0">%s</ul>
        <p style="font-size:12px;color:#94A3B8;margin-top:28px">Sent by %s. You are receiving this because you are an admin of %s. We never ask for your password by email.</p>
      </td></tr>
    </table>
  </td></tr>
</table>
`, escHTML(orgName), escHTML(bsonStr(u, "name", bsonStr(u, "email", ""))), rowsHTML,
			len(overdue), overdueHTML, escHTML(cfg.EmailFromName), escHTML(orgName))
		_, _ = sendEmail(asStr(u["email"]), subject, htmlBody)
	}
}
