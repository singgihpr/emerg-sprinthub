"""Invite + SMTP unit checks. No network, no Mongo: smtplib and db are faked.

Run: cd backend && .venv/bin/python -m pytest tests/test_invite_smtp.py
"""
import asyncio
import hashlib
import os
import sys
from datetime import timedelta

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
os.environ.setdefault("MONGO_URL", "mongodb://localhost:27017")
os.environ.setdefault("DB_NAME", "sprinthub_test")
os.environ.setdefault("JWT_SECRET", "test-secret")
os.environ.setdefault("SMTP_HOST", "smtp.test")
os.environ.setdefault("SMTP_PORT", "587")
os.environ.setdefault("SMTP_USER", "user@test")
os.environ.setdefault("SMTP_PASS", "pass")
os.environ.setdefault("EMAIL_FROM", "no-reply@test")
os.environ.setdefault("APP_BASE_URL", "https://app.test")

import server  # noqa: E402  (must import after env is set)
from fastapi import HTTPException, Response  # noqa: E402


# ---------------------- fakes ----------------------
class FakeSMTP:
    instances = []

    def __init__(self, host, port, timeout=None):
        self.host, self.port = host, port
        self.calls = []
        FakeSMTP.instances.append(self)

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False

    def starttls(self):
        self.calls.append("starttls")

    def login(self, u, p):
        self.calls.append(("login", u, p))

    def send_message(self, msg):
        self.calls.append(("send", msg))


class _Result:
    def __init__(self, n):
        self.deleted_count = n


class _Cursor:
    def __init__(self, docs):
        self.docs = docs

    async def to_list(self, n=None):
        return [dict(d) for d in self.docs]


def _match(doc, q):
    for k, v in q.items():
        if isinstance(v, dict) and "$in" in v:
            if doc.get(k) not in v["$in"]:
                return False
        elif doc.get(k) != v:
            return False
    return True


class _Coll:
    def __init__(self):
        self.docs = []

    async def find_one(self, q, proj=None):
        return next((dict(d) for d in self.docs if _match(d, q)), None)

    def find(self, q, proj=None):
        return _Cursor([d for d in self.docs if _match(d, q)])

    async def insert_one(self, doc):
        self.docs.append(dict(doc))

    async def delete_one(self, q):
        for i, d in enumerate(self.docs):
            if _match(d, q):
                self.docs.pop(i)
                return _Result(1)
        return _Result(0)

    async def delete_many(self, q):
        keep = [d for d in self.docs if not _match(d, q)]
        n = len(self.docs) - len(keep)
        self.docs = keep
        return _Result(n)

    async def update_one(self, q, upd):
        for d in self.docs:
            if _match(d, q):
                d.update(upd["$set"])
                return


class _DB:
    def __init__(self):
        self.users = _Coll()
        self.memberships = _Coll()
        self.organizations = _Coll()
        self.invites = _Coll()


def _sent_msg(inst):
    return next(c[1] for c in inst.calls if c[0] == "send")


@pytest.fixture(autouse=True)
def _reset_fake_smtp(monkeypatch):
    FakeSMTP.instances.clear()
    monkeypatch.setattr(server.smtplib, "SMTP", FakeSMTP)
    monkeypatch.setattr(server, "SMTP_HOST", "smtp.test")
    monkeypatch.setattr(server, "SMTP_USER", "user@test")
    monkeypatch.setattr(server, "SMTP_PASS", "pass")


# ---------------------- send_email: SMTP transport ----------------------
def test_smtp_send_with_creds_starttls_login():
    html = '<p>Hi</p><a href="https://app.test/accept-invite/tok">Set password</a>'
    mid = asyncio.run(server.send_email(to="jane@x.com", subject="Invite", html=html))
    assert mid
    inst = FakeSMTP.instances[0]
    assert "starttls" in inst.calls
    assert ("login", "user@test", "pass") in inst.calls
    msg = _sent_msg(inst)
    assert msg["To"] == "jane@x.com"
    assert msg["Subject"] == "Invite"
    assert "https://app.test/accept-invite/tok" in msg.get_body(preferencelist=("html",)).get_content()


def test_smtp_no_creds_plain_send(monkeypatch):
    monkeypatch.setattr(server, "SMTP_USER", None)
    monkeypatch.setattr(server, "SMTP_PASS", None)
    assert asyncio.run(server.send_email(to="j@x.com", subject="s", html="<p>x</p>"))
    calls = FakeSMTP.instances[0].calls
    assert "starttls" not in calls
    assert not any(c[0] == "login" for c in calls if isinstance(c, tuple))


def test_smtp_failure_returns_none(monkeypatch):
    def boom(*a, **k):
        raise OSError("smtp down")
    monkeypatch.setattr(server.smtplib, "SMTP", boom)
    assert asyncio.run(server.send_email(to="j@x.com", subject="s", html="<p>x</p>")) is None


def test_no_transport_configured_returns_none(monkeypatch):
    monkeypatch.setattr(server, "SMTP_HOST", None)
    monkeypatch.setattr(server, "EMAIL_KEY", None)
    assert asyncio.run(server.send_email(to="j@x.com", subject="s", html="<p>x</p>")) is None


def test_http_link_blocked_by_scanner():
    html = '<a href="http://app.test/x">app.test/x</a>'
    assert asyncio.run(server.send_email(to="j@x.com", subject="s", html=html)) is None
    assert not FakeSMTP.instances


# ---------------------- _invite_email builder ----------------------
def test_invite_email_with_token():
    subject, html = server._invite_email(name="Jane", inviter_name="Bob", org_name="Acme", token="tok123")
    assert "Acme" in subject
    server._assert_safe_email(subject, html)  # must not raise
    assert "https://app.test/accept-invite/tok123" in html
    assert "Jane" in html and "Bob" in html and "7 days" in html
    assert "Welcome@123" not in html


def test_invite_email_without_token_has_no_accept_link():
    subject, html = server._invite_email(name="Jane", inviter_name="Bob", org_name="Acme", token=None)
    server._assert_safe_email(subject, html)
    assert "accept-invite" not in html


def test_invite_email_non_https_base_omits_button(monkeypatch):
    monkeypatch.setattr(server, "APP_BASE_URL", "http://localhost:8030")
    _, html = server._invite_email(name="J", inviter_name="B", org_name="O", token="t")
    assert "<a " not in html


def test_invite_email_escapes_input():
    _, html = server._invite_email(name="<script>", inviter_name="B", org_name="O", token=None)
    assert "<script>" not in html
    assert "&lt;script&gt;" in html


def test_hash_token_is_sha256():
    assert server._hash_token("abc") == hashlib.sha256(b"abc").hexdigest()


# ---------------------- invite endpoints with fake db ----------------------
def _seed_db(monkeypatch, expires_in=timedelta(days=1), token="good-token"):
    db = _DB()
    db.invites.docs.append({
        "invite_id": "inv_1", "token_hash": server._hash_token(token),
        "org_id": "org_1", "email": "jane@x.com", "name": "Jane", "role": "member",
        "invited_by": "u_admin", "created_at": server.now_utc().isoformat(),
        "expires_at": server.now_utc() + expires_in,
    })
    db.users.docs.append({"user_id": "u_1", "email": "jane@x.com", "name": "Jane",
                          "password_hash": None, "picture": None})
    db.organizations.docs.append({"org_id": "org_1", "name": "Acme"})
    monkeypatch.setattr(server, "db", db)
    return db


def test_get_invite_ok_and_404(monkeypatch):
    _seed_db(monkeypatch)
    info = asyncio.run(server.get_invite("good-token"))
    assert info == {"email": "jane@x.com", "name": "Jane", "org_name": "Acme", "role": "member"}
    with pytest.raises(HTTPException) as e:
        asyncio.run(server.get_invite("bogus"))
    assert e.value.status_code == 404


def test_accept_invite_sets_password_single_use(monkeypatch):
    db = _seed_db(monkeypatch)
    out = asyncio.run(server.accept_invite("good-token", server.InviteAccept(password="secret123"), Response()))
    assert out["user_id"] == "u_1" and out["token"]
    assert server.verify_password("secret123", db.users.docs[0]["password_hash"])
    assert db.invites.docs == []
    with pytest.raises(HTTPException):  # reuse
        asyncio.run(server.accept_invite("good-token", server.InviteAccept(password="secret123"), Response()))


def test_accept_invite_expired_404(monkeypatch):
    _seed_db(monkeypatch, expires_in=timedelta(days=-1))
    with pytest.raises(HTTPException) as e:
        asyncio.run(server.accept_invite("good-token", server.InviteAccept(password="secret123"), Response()))
    assert e.value.status_code == 404


def test_accept_invite_rejects_short_password():
    with pytest.raises(Exception):  # pydantic min_length=6
        server.InviteAccept(password="abc")


def test_list_members_invited_flag_no_hash_leak(monkeypatch):
    db = _seed_db(monkeypatch)
    db.memberships.docs += [
        {"membership_id": "m1", "org_id": "org_1", "user_id": "u_admin", "role": "owner"},
        {"membership_id": "m2", "org_id": "org_1", "user_id": "u_1", "role": "member"},
        {"membership_id": "m3", "org_id": "org_1", "user_id": "u_g", "role": "member"},
    ]
    db.users.docs += [
        {"user_id": "u_admin", "email": "a@x.com", "name": "A", "password_hash": "hashed", "picture": None},
        {"user_id": "u_g", "email": "g@x.com", "name": "G", "password_hash": None,
         "picture": None, "auth_provider": "google"},
    ]
    members = asyncio.run(server.list_members("org_1", {"user_id": "u_admin"}))
    by_id = {m["user_id"]: m for m in members}
    assert by_id["u_admin"]["invited"] is False
    assert by_id["u_1"]["invited"] is True
    assert by_id["u_g"]["invited"] is False  # Google login, not pending
    assert all("password_hash" not in m for m in members)


if __name__ == "__main__":
    sys.exit(pytest.main([__file__, "-q"]))
