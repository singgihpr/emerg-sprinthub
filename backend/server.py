from dotenv import load_dotenv
from pathlib import Path
ROOT_DIR = Path(__file__).parent
load_dotenv(ROOT_DIR / '.env')

import os
import uuid
import logging
import bcrypt
import jwt
import httpx
from datetime import datetime, timezone, timedelta
from typing import List, Optional
from fastapi import FastAPI, APIRouter, HTTPException, Request, Response, Depends
from fastapi.responses import JSONResponse
from starlette.middleware.cors import CORSMiddleware
from motor.motor_asyncio import AsyncIOMotorClient
from pydantic import BaseModel, Field, EmailStr

# ----------------------
# Setup
# ----------------------
mongo_url = os.environ['MONGO_URL']
client = AsyncIOMotorClient(mongo_url)
db = client[os.environ['DB_NAME']]

JWT_SECRET = os.environ['JWT_SECRET']
JWT_ALG = "HS256"

app = FastAPI(title="Sprint Hub API")
api = APIRouter(prefix="/api")

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("sprinthub")

# ----------------------
# Helpers
# ----------------------
def new_id(prefix: str = "id") -> str:
    return f"{prefix}_{uuid.uuid4().hex[:12]}"

def now_utc() -> datetime:
    return datetime.now(timezone.utc)

def iso(dt: Optional[datetime]) -> Optional[str]:
    if dt is None:
        return None
    if isinstance(dt, str):
        return dt
    return dt.isoformat()

def hash_password(p: str) -> str:
    return bcrypt.hashpw(p.encode(), bcrypt.gensalt()).decode()

def verify_password(p: str, h: str) -> bool:
    try:
        return bcrypt.checkpw(p.encode(), h.encode())
    except Exception:
        return False

def create_access_token(user_id: str, email: str) -> str:
    payload = {
        "sub": user_id, "email": email, "type": "access",
        "exp": now_utc() + timedelta(days=7)
    }
    return jwt.encode(payload, JWT_SECRET, algorithm=JWT_ALG)

async def get_current_user(request: Request) -> dict:
    token = request.cookies.get("access_token")
    if not token:
        auth = request.headers.get("Authorization", "")
        if auth.startswith("Bearer "):
            token = auth[7:]
    if not token:
        # try session_token cookie for google flow
        st = request.cookies.get("session_token")
        if st:
            session = await db.user_sessions.find_one({"session_token": st}, {"_id": 0})
            if session:
                exp = session.get("expires_at")
                if isinstance(exp, str):
                    exp = datetime.fromisoformat(exp)
                if exp and exp.tzinfo is None:
                    exp = exp.replace(tzinfo=timezone.utc)
                if exp and exp > now_utc():
                    user = await db.users.find_one({"user_id": session["user_id"]}, {"_id": 0, "password_hash": 0})
                    if user:
                        return user
        raise HTTPException(status_code=401, detail="Not authenticated")
    try:
        payload = jwt.decode(token, JWT_SECRET, algorithms=[JWT_ALG])
        user = await db.users.find_one({"user_id": payload["sub"]}, {"_id": 0, "password_hash": 0})
        if not user:
            raise HTTPException(status_code=401, detail="User not found")
        return user
    except jwt.ExpiredSignatureError:
        raise HTTPException(status_code=401, detail="Token expired")
    except jwt.InvalidTokenError:
        raise HTTPException(status_code=401, detail="Invalid token")

def set_auth_cookie(response: Response, token: str):
    response.set_cookie(
        key="access_token", value=token,
        httponly=True, secure=True, samesite="none",
        max_age=7 * 24 * 60 * 60, path="/"
    )

async def insert_and_get(coll, doc: dict) -> dict:
    """Insert a copy so the returned dict has no ObjectId `_id`."""
    await coll.insert_one(dict(doc))
    doc.pop("_id", None)
    return doc


# ----------------------
# Models
# ----------------------
class RegisterBody(BaseModel):
    email: EmailStr
    password: str
    name: str

class LoginBody(BaseModel):
    email: EmailStr
    password: str

class OrgCreate(BaseModel):
    name: str
    logo: Optional[str] = None

class OrgUpdate(BaseModel):
    name: Optional[str] = None
    logo: Optional[str] = None

class ProfileUpdate(BaseModel):
    name: Optional[str] = None
    picture: Optional[str] = None

class PasswordChange(BaseModel):
    current_password: Optional[str] = None
    new_password: str = Field(..., min_length=6, max_length=128)

class MemberInvite(BaseModel):
    email: EmailStr
    name: str
    role: str = "member"  # owner/admin/manager/member

class ProjectCreate(BaseModel):
    name: str
    key: str
    description: Optional[str] = ""
    color: Optional[str] = "#4F46E5"

class ProjectUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None
    color: Optional[str] = None

class ProjectMemberAdd(BaseModel):
    user_id: str
    role: Optional[str] = "member"  # member/lead

class SprintCreate(BaseModel):
    project_id: str
    name: str
    goal: Optional[str] = ""
    start_date: Optional[str] = None
    end_date: Optional[str] = None

class TaskCreate(BaseModel):
    project_id: str
    title: str
    description: Optional[str] = ""
    status: str = "todo"  # todo/in_progress/review/done
    priority: str = "medium"  # low/medium/high/urgent
    type: str = "task"  # task/routine/bug/story
    assignee_id: Optional[str] = None
    sprint_id: Optional[str] = None
    start_date: Optional[str] = None
    due_date: Optional[str] = None
    estimate_hours: Optional[float] = 0

class TaskUpdate(BaseModel):
    title: Optional[str] = None
    description: Optional[str] = None
    status: Optional[str] = None
    priority: Optional[str] = None
    assignee_id: Optional[str] = None
    sprint_id: Optional[str] = None
    start_date: Optional[str] = None
    due_date: Optional[str] = None
    estimate_hours: Optional[float] = None

class TimeEntryCreate(BaseModel):
    task_id: str
    minutes: int
    note: Optional[str] = ""
    date: Optional[str] = None

class CommentCreate(BaseModel):
    body: str = Field(..., min_length=1, max_length=5000)

# ----------------------
# Auth endpoints
# ----------------------
@api.post("/auth/register")
async def register(body: RegisterBody, response: Response):
    email = body.email.lower()
    existing = await db.users.find_one({"email": email})
    if existing:
        raise HTTPException(status_code=400, detail="Email already registered")
    user_id = new_id("user")
    doc = {
        "user_id": user_id,
        "email": email,
        "name": body.name,
        "password_hash": hash_password(body.password),
        "picture": None,
        "created_at": now_utc().isoformat(),
    }
    await db.users.insert_one(doc)
    # Create default org for new user
    org_id = new_id("org")
    await db.organizations.insert_one({
        "org_id": org_id, "name": f"{body.name}'s Workspace",
        "owner_id": user_id, "logo": None, "created_at": now_utc().isoformat()
    })
    await db.memberships.insert_one({
        "membership_id": new_id("mem"), "org_id": org_id,
        "user_id": user_id, "role": "owner", "created_at": now_utc().isoformat()
    })
    token = create_access_token(user_id, email)
    set_auth_cookie(response, token)
    return {"user_id": user_id, "email": email, "name": body.name, "token": token}

@api.post("/auth/login")
async def login(body: LoginBody, response: Response):
    email = body.email.lower()
    user = await db.users.find_one({"email": email})
    if not user or not user.get("password_hash") or not verify_password(body.password, user["password_hash"]):
        raise HTTPException(status_code=401, detail="Invalid email or password")
    token = create_access_token(user["user_id"], email)
    set_auth_cookie(response, token)
    return {
        "user_id": user["user_id"], "email": email,
        "name": user.get("name"), "picture": user.get("picture"),
        "token": token
    }

@api.post("/auth/logout")
async def logout(response: Response):
    response.delete_cookie("access_token", path="/")
    response.delete_cookie("session_token", path="/")
    return {"ok": True}

@api.get("/auth/me")
async def me(user: dict = Depends(get_current_user)):
    return user

@api.patch("/auth/me")
async def update_profile(body: ProfileUpdate, user: dict = Depends(get_current_user)):
    updates = body.model_dump(exclude_unset=True)
    if not updates:
        raise HTTPException(status_code=400, detail="No fields to update")
    await db.users.update_one({"user_id": user["user_id"]}, {"$set": updates})
    return await db.users.find_one({"user_id": user["user_id"]}, {"_id": 0, "password_hash": 0})

@api.post("/auth/change-password")
async def change_password(body: PasswordChange, user: dict = Depends(get_current_user)):
    u = await db.users.find_one({"user_id": user["user_id"]})
    if not u:
        raise HTTPException(status_code=404, detail="User not found")
    # If user already has a password, verify current
    if u.get("password_hash"):
        if not body.current_password or not verify_password(body.current_password, u["password_hash"]):
            raise HTTPException(status_code=400, detail="Current password is incorrect")
    await db.users.update_one({"user_id": user["user_id"]}, {"$set": {"password_hash": hash_password(body.new_password)}})
    return {"ok": True}

@api.post("/auth/google/session")
async def google_session(request: Request, response: Response):
    body = await request.json()
    session_id = body.get("session_id")
    if not session_id:
        raise HTTPException(status_code=400, detail="session_id required")
    async with httpx.AsyncClient(timeout=15) as hc:
        r = await hc.get(
            "https://demobackend.emergentagent.com/auth/v1/env/oauth/session-data",
            headers={"X-Session-ID": session_id}
        )
    if r.status_code != 200:
        raise HTTPException(status_code=401, detail="Invalid session")
    data = r.json()
    email = data["email"].lower()
    user = await db.users.find_one({"email": email})
    if not user:
        user_id = new_id("user")
        user_doc = {
            "user_id": user_id, "email": email, "name": data.get("name", email),
            "picture": data.get("picture"), "password_hash": None,
            "created_at": now_utc().isoformat()
        }
        await db.users.insert_one(user_doc)
        # default org
        org_id = new_id("org")
        await db.organizations.insert_one({
            "org_id": org_id, "name": f"{data.get('name', 'My')}'s Workspace",
            "owner_id": user_id, "logo": None, "created_at": now_utc().isoformat()
        })
        await db.memberships.insert_one({
            "membership_id": new_id("mem"), "org_id": org_id,
            "user_id": user_id, "role": "owner", "created_at": now_utc().isoformat()
        })
    else:
        user_id = user["user_id"]
        await db.users.update_one({"user_id": user_id}, {"$set": {"picture": data.get("picture"), "name": data.get("name", user.get("name"))}})
    # Store session
    session_token = data["session_token"]
    await db.user_sessions.insert_one({
        "user_id": user_id, "session_token": session_token,
        "expires_at": (now_utc() + timedelta(days=7)).isoformat(),
        "created_at": now_utc().isoformat()
    })
    response.set_cookie(
        key="session_token", value=session_token,
        httponly=True, secure=True, samesite="none",
        max_age=7 * 24 * 60 * 60, path="/"
    )
    # Also set access_token JWT for unified auth
    jwt_tok = create_access_token(user_id, email)
    set_auth_cookie(response, jwt_tok)
    return {"user_id": user_id, "email": email, "name": data.get("name"), "picture": data.get("picture"), "token": jwt_tok}

# ----------------------
# Organizations
# ----------------------
@api.get("/orgs")
async def list_orgs(user: dict = Depends(get_current_user)):
    memberships = await db.memberships.find({"user_id": user["user_id"]}, {"_id": 0}).to_list(500)
    org_ids = [m["org_id"] for m in memberships]
    orgs = await db.organizations.find({"org_id": {"$in": org_ids}}, {"_id": 0}).sort("created_at", 1).to_list(500)
    role_map = {m["org_id"]: m["role"] for m in memberships}
    for o in orgs:
        o["role"] = role_map.get(o["org_id"])
    return orgs

@api.post("/orgs")
async def create_org(body: OrgCreate, user: dict = Depends(get_current_user)):
    org_id = new_id("org")
    doc = {"org_id": org_id, "name": body.name, "owner_id": user["user_id"],
           "logo": body.logo, "created_at": now_utc().isoformat()}
    await db.organizations.insert_one(dict(doc))
    await db.memberships.insert_one({
        "membership_id": new_id("mem"), "org_id": org_id,
        "user_id": user["user_id"], "role": "owner", "created_at": now_utc().isoformat()
    })
    doc["role"] = "owner"
    doc.pop("_id", None)
    return doc

@api.patch("/orgs/{org_id}")
async def update_org(org_id: str, body: OrgUpdate, user: dict = Depends(get_current_user)):
    m = await db.memberships.find_one({"org_id": org_id, "user_id": user["user_id"]}, {"_id": 0})
    if not m:
        raise HTTPException(status_code=403, detail="Not a member of this organization")
    if m["role"] not in ("owner", "admin"):
        raise HTTPException(status_code=403, detail="Only owner or admin can edit workspace")
    updates = body.model_dump(exclude_unset=True)
    if not updates:
        raise HTTPException(status_code=400, detail="No fields to update")
    result = await db.organizations.update_one({"org_id": org_id}, {"$set": updates})
    if result.matched_count == 0:
        raise HTTPException(status_code=404, detail="Workspace not found")
    org = await db.organizations.find_one({"org_id": org_id}, {"_id": 0})
    org["role"] = m["role"]
    return org


async def ensure_member(org_id: str, user_id: str) -> dict:
    m = await db.memberships.find_one({"org_id": org_id, "user_id": user_id}, {"_id": 0})
    if not m:
        raise HTTPException(status_code=403, detail="Not a member of this organization")
    return m

# ----------------------
# Members
# ----------------------
@api.get("/orgs/{org_id}/members")
async def list_members(org_id: str, user: dict = Depends(get_current_user)):
    await ensure_member(org_id, user["user_id"])
    members = await db.memberships.find({"org_id": org_id}, {"_id": 0}).to_list(500)
    user_ids = [m["user_id"] for m in members]
    users = await db.users.find({"user_id": {"$in": user_ids}}, {"_id": 0, "password_hash": 0}).to_list(500)
    umap = {u["user_id"]: u for u in users}
    out = []
    for m in members:
        u = umap.get(m["user_id"], {})
        out.append({**u, "role": m["role"], "membership_id": m["membership_id"]})
    return out

@api.post("/orgs/{org_id}/members")
async def invite_member(org_id: str, body: MemberInvite, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    if m["role"] not in ("owner", "admin"):
        raise HTTPException(status_code=403, detail="Insufficient permissions")
    email = body.email.lower()
    existing = await db.users.find_one({"email": email})
    if existing:
        target_id = existing["user_id"]
    else:
        target_id = new_id("user")
        await db.users.insert_one({
            "user_id": target_id, "email": email, "name": body.name,
            "password_hash": hash_password("Welcome@123"), "picture": None,
            "created_at": now_utc().isoformat()
        })
    already = await db.memberships.find_one({"org_id": org_id, "user_id": target_id})
    if already:
        raise HTTPException(status_code=400, detail="User already a member")
    await db.memberships.insert_one({
        "membership_id": new_id("mem"), "org_id": org_id,
        "user_id": target_id, "role": body.role, "created_at": now_utc().isoformat()
    })
    return {"ok": True, "user_id": target_id}

# ----------------------
# Projects
# ----------------------
async def _get_accessible_project_ids(org_id: str, user_id: str, role: str) -> Optional[List[str]]:
    """Return None if user can access ALL projects (owner/admin); otherwise the list of project_ids they belong to."""
    if role in ("owner", "admin"):
        return None
    pm = await db.project_members.find({"org_id": org_id, "user_id": user_id}, {"_id": 0}).to_list(500)
    return [p["project_id"] for p in pm]

async def require_project_access(org_id: str, project_id: str, user: dict) -> dict:
    """Ensure user is org member AND can access the given project. Returns membership."""
    m = await ensure_member(org_id, user["user_id"])
    if m["role"] not in ("owner", "admin"):
        pm = await db.project_members.find_one({"org_id": org_id, "project_id": project_id, "user_id": user["user_id"]})
        if not pm:
            raise HTTPException(status_code=403, detail="No access to this project")
    return m

async def require_task_access(org_id: str, task_id: str, user: dict) -> dict:
    """Fetch task and check project access. Returns the task doc."""
    task = await db.tasks.find_one({"task_id": task_id, "org_id": org_id}, {"_id": 0})
    if not task:
        raise HTTPException(status_code=404, detail="Task not found")
    await require_project_access(org_id, task["project_id"], user)
    return task

@api.get("/orgs/{org_id}/projects")
async def list_projects(org_id: str, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    allowed = await _get_accessible_project_ids(org_id, user["user_id"], m["role"])
    q = {"org_id": org_id}
    if allowed is not None:
        q["project_id"] = {"$in": allowed}
    return await db.projects.find(q, {"_id": 0}).to_list(500)

@api.post("/orgs/{org_id}/projects")
async def create_project(org_id: str, body: ProjectCreate, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    if m["role"] not in ("owner", "admin", "manager"):
        raise HTTPException(status_code=403, detail="Requires manager or higher role")
    doc = {
        "project_id": new_id("prj"), "org_id": org_id,
        "name": body.name, "key": body.key.upper(),
        "description": body.description, "color": body.color,
        "created_by": user["user_id"], "created_at": now_utc().isoformat()
    }
    await db.projects.insert_one(dict(doc))
    # creator becomes a project member (as lead)
    await db.project_members.insert_one({
        "org_id": org_id, "project_id": doc["project_id"], "user_id": user["user_id"],
        "role": "lead", "added_at": now_utc().isoformat(), "added_by": user["user_id"]
    })
    doc.pop("_id", None)
    return doc

@api.patch("/orgs/{org_id}/projects/{project_id}")
async def update_project(org_id: str, project_id: str, body: ProjectUpdate, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    if m["role"] not in ("owner", "admin", "manager"):
        raise HTTPException(status_code=403, detail="Requires manager or higher role")
    updates = {k: v for k, v in body.model_dump().items() if v is not None}
    if not updates:
        raise HTTPException(status_code=400, detail="No fields to update")
    result = await db.projects.update_one({"project_id": project_id, "org_id": org_id}, {"$set": updates})
    if result.matched_count == 0:
        raise HTTPException(status_code=404, detail="Project not found")
    return await db.projects.find_one({"project_id": project_id}, {"_id": 0})

@api.get("/orgs/{org_id}/projects/{project_id}/members")
async def list_project_members(org_id: str, project_id: str, user: dict = Depends(get_current_user)):
    await require_project_access(org_id, project_id, user)
    members = await db.project_members.find({"org_id": org_id, "project_id": project_id}, {"_id": 0}).to_list(500)
    user_ids = [m["user_id"] for m in members]
    users = await db.users.find({"user_id": {"$in": user_ids}}, {"_id": 0, "password_hash": 0}).to_list(500)
    umap = {u["user_id"]: u for u in users}
    out = []
    for m in members:
        u = umap.get(m["user_id"], {})
        out.append({**u, "project_role": m.get("role", "member"), "added_at": m.get("added_at")})
    return out

@api.post("/orgs/{org_id}/projects/{project_id}/members")
async def add_project_member(org_id: str, project_id: str, body: ProjectMemberAdd, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    if m["role"] not in ("owner", "admin", "manager"):
        raise HTTPException(status_code=403, detail="Requires manager or higher role")
    # user must be an org member
    target = await db.memberships.find_one({"org_id": org_id, "user_id": body.user_id})
    if not target:
        raise HTTPException(status_code=400, detail="User is not a member of the organization")
    existing = await db.project_members.find_one({"org_id": org_id, "project_id": project_id, "user_id": body.user_id})
    if existing:
        raise HTTPException(status_code=400, detail="Already a project member")
    doc = {
        "org_id": org_id, "project_id": project_id, "user_id": body.user_id,
        "role": body.role or "member", "added_at": now_utc().isoformat(),
        "added_by": user["user_id"]
    }
    await db.project_members.insert_one(dict(doc))
    return {"ok": True}

@api.delete("/orgs/{org_id}/projects/{project_id}/members/{user_id}")
async def remove_project_member(org_id: str, project_id: str, user_id: str, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    if m["role"] not in ("owner", "admin", "manager"):
        raise HTTPException(status_code=403, detail="Requires manager or higher role")
    await db.project_members.delete_one({"org_id": org_id, "project_id": project_id, "user_id": user_id})
    return {"ok": True}

# ----------------------
# Sprints
# ----------------------
@api.get("/orgs/{org_id}/sprints")
async def list_sprints(org_id: str, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    allowed = await _get_accessible_project_ids(org_id, user["user_id"], m["role"])
    q = {"org_id": org_id}
    if allowed is not None:
        q["project_id"] = {"$in": allowed}
    return await db.sprints.find(q, {"_id": 0}).sort("created_at", -1).to_list(500)

@api.post("/orgs/{org_id}/sprints")
async def create_sprint(org_id: str, body: SprintCreate, user: dict = Depends(get_current_user)):
    m = await require_project_access(org_id, body.project_id, user)
    if m["role"] not in ("owner", "admin", "manager"):
        raise HTTPException(status_code=403, detail="Requires manager or higher role")
    doc = {
        "sprint_id": new_id("spr"), "org_id": org_id, "project_id": body.project_id,
        "name": body.name, "goal": body.goal,
        "start_date": body.start_date, "end_date": body.end_date,
        "status": "planned", "created_at": now_utc().isoformat()
    }
    await db.sprints.insert_one(dict(doc))
    doc.pop("_id", None)
    return doc

# ----------------------
# Tasks
# ----------------------
@api.get("/orgs/{org_id}/tasks")
async def list_tasks(org_id: str, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    allowed = await _get_accessible_project_ids(org_id, user["user_id"], m["role"])
    q = {"org_id": org_id}
    if allowed is not None:
        q["project_id"] = {"$in": allowed}
    return await db.tasks.find(q, {"_id": 0}).sort("created_at", -1).to_list(2000)

@api.post("/orgs/{org_id}/tasks")
async def create_task(org_id: str, body: TaskCreate, user: dict = Depends(get_current_user)):
    await require_project_access(org_id, body.project_id, user)
    doc = {
        "task_id": new_id("tsk"), "org_id": org_id,
        "project_id": body.project_id, "title": body.title,
        "description": body.description, "status": body.status,
        "priority": body.priority, "type": body.type,
        "assignee_id": body.assignee_id, "sprint_id": body.sprint_id,
        "start_date": body.start_date, "due_date": body.due_date,
        "estimate_hours": body.estimate_hours or 0,
        "logged_minutes": 0,
        "created_by": user["user_id"], "created_at": now_utc().isoformat(),
        "updated_at": now_utc().isoformat(),
        "completed_at": None
    }
    await db.tasks.insert_one(dict(doc))
    doc.pop("_id", None)
    return doc

@api.patch("/orgs/{org_id}/tasks/{task_id}")
async def update_task(org_id: str, task_id: str, body: TaskUpdate, user: dict = Depends(get_current_user)):
    await require_task_access(org_id, task_id, user)
    updates = {k: v for k, v in body.model_dump().items() if v is not None}
    updates["updated_at"] = now_utc().isoformat()
    if updates.get("status") == "done":
        updates["completed_at"] = now_utc().isoformat()
    result = await db.tasks.update_one({"task_id": task_id, "org_id": org_id}, {"$set": updates})
    if result.matched_count == 0:
        raise HTTPException(status_code=404, detail="Task not found")
    return await db.tasks.find_one({"task_id": task_id}, {"_id": 0})

@api.delete("/orgs/{org_id}/tasks/{task_id}")
async def delete_task(org_id: str, task_id: str, user: dict = Depends(get_current_user)):
    await require_task_access(org_id, task_id, user)
    await db.tasks.delete_one({"task_id": task_id, "org_id": org_id})
    await db.time_entries.delete_many({"task_id": task_id})
    return {"ok": True}

# ----------------------
# Time tracking
# ----------------------
@api.get("/orgs/{org_id}/time-entries")
async def list_time_entries(org_id: str, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    allowed = await _get_accessible_project_ids(org_id, user["user_id"], m["role"])
    q = {"org_id": org_id}
    if allowed is not None:
        tasks = await db.tasks.find({"org_id": org_id, "project_id": {"$in": allowed}}, {"_id": 0}).to_list(5000)
        q["task_id"] = {"$in": [t["task_id"] for t in tasks]}
    return await db.time_entries.find(q, {"_id": 0}).sort("created_at", -1).to_list(2000)

@api.post("/orgs/{org_id}/time-entries")
async def create_time_entry(org_id: str, body: TimeEntryCreate, user: dict = Depends(get_current_user)):
    await require_task_access(org_id, body.task_id, user)
    doc = {
        "entry_id": new_id("te"), "org_id": org_id,
        "task_id": body.task_id, "user_id": user["user_id"],
        "minutes": body.minutes, "note": body.note,
        "date": body.date or now_utc().date().isoformat(),
        "created_at": now_utc().isoformat()
    }
    await db.time_entries.insert_one(dict(doc))
    await db.tasks.update_one({"task_id": body.task_id, "org_id": org_id}, {"$inc": {"logged_minutes": body.minutes}})
    doc.pop("_id", None)
    return doc

# active timer
@api.get("/orgs/{org_id}/timer")
async def get_timer(org_id: str, user: dict = Depends(get_current_user)):
    await ensure_member(org_id, user["user_id"])
    t = await db.active_timers.find_one({"org_id": org_id, "user_id": user["user_id"]}, {"_id": 0})
    return t or {}

@api.post("/orgs/{org_id}/timer/start")
async def start_timer(org_id: str, request: Request, user: dict = Depends(get_current_user)):
    body = await request.json()
    task_id = body.get("task_id")
    if not task_id:
        raise HTTPException(status_code=400, detail="task_id required")
    await require_task_access(org_id, task_id, user)
    await db.active_timers.delete_many({"org_id": org_id, "user_id": user["user_id"]})
    doc = {
        "timer_id": new_id("tmr"), "org_id": org_id,
        "user_id": user["user_id"], "task_id": task_id,
        "started_at": now_utc().isoformat()
    }
    await db.active_timers.insert_one(dict(doc))
    doc.pop("_id", None)
    return doc

@api.post("/orgs/{org_id}/timer/stop")
async def stop_timer(org_id: str, user: dict = Depends(get_current_user)):
    await ensure_member(org_id, user["user_id"])
    t = await db.active_timers.find_one({"org_id": org_id, "user_id": user["user_id"]}, {"_id": 0})
    if not t:
        raise HTTPException(status_code=404, detail="No active timer")
    started = datetime.fromisoformat(t["started_at"])
    if started.tzinfo is None:
        started = started.replace(tzinfo=timezone.utc)
    minutes = max(1, int((now_utc() - started).total_seconds() // 60))
    entry = {
        "entry_id": new_id("te"), "org_id": org_id,
        "task_id": t["task_id"], "user_id": user["user_id"],
        "minutes": minutes, "note": "Timer",
        "date": now_utc().date().isoformat(),
        "created_at": now_utc().isoformat()
    }
    await db.time_entries.insert_one(dict(entry))
    await db.tasks.update_one({"task_id": t["task_id"], "org_id": org_id}, {"$inc": {"logged_minutes": minutes}})
    await db.active_timers.delete_many({"org_id": org_id, "user_id": user["user_id"]})
    entry.pop("_id", None)
    return entry

# ----------------------
# Team Activity (admin/manager only)
# ----------------------
@api.get("/orgs/{org_id}/team-activity")
async def team_activity(org_id: str, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    if m["role"] not in ("owner", "admin", "manager"):
        raise HTTPException(status_code=403, detail="Requires admin or manager role")

    memberships = await db.memberships.find({"org_id": org_id}, {"_id": 0}).to_list(500)
    user_ids = [x["user_id"] for x in memberships]
    users = await db.users.find({"user_id": {"$in": user_ids}}, {"_id": 0, "password_hash": 0}).to_list(500)
    umap = {u["user_id"]: u for u in users}
    role_map = {x["user_id"]: x["role"] for x in memberships}

    tasks = await db.tasks.find({"org_id": org_id}, {"_id": 0}).to_list(5000)
    active_timers = await db.active_timers.find({"org_id": org_id}, {"_id": 0}).to_list(500)
    timer_map = {t["user_id"]: t for t in active_timers}
    task_map = {t["task_id"]: t for t in tasks}
    projects = await db.projects.find({"org_id": org_id}, {"_id": 0}).to_list(500)
    project_map = {p["project_id"]: p for p in projects}

    today_iso = now_utc().date().isoformat()
    week_start = (now_utc().date() - timedelta(days=6)).isoformat()

    entries = await db.time_entries.find(
        {"org_id": org_id, "date": {"$gte": week_start}},
        {"_id": 0}
    ).sort("created_at", -1).to_list(2000)

    now_ts = now_utc()

    rows = []
    for uid in user_ids:
        u = umap.get(uid, {})
        my_tasks = [t for t in tasks if t.get("assignee_id") == uid]
        active_tasks = [t for t in my_tasks if t.get("status") != "done"]
        in_progress = [t for t in my_tasks if t.get("status") == "in_progress"]

        my_entries = [e for e in entries if e.get("user_id") == uid]
        today_min = sum(e["minutes"] for e in my_entries if e.get("date") == today_iso)
        week_min = sum(e["minutes"] for e in my_entries)

        # active timer
        tmr = timer_map.get(uid)
        active_timer = None
        if tmr:
            started = datetime.fromisoformat(tmr["started_at"])
            if started.tzinfo is None:
                started = started.replace(tzinfo=timezone.utc)
            elapsed = max(0, int((now_ts - started).total_seconds()))
            t_task = task_map.get(tmr["task_id"])
            active_timer = {
                "task_id": tmr["task_id"],
                "task_title": t_task.get("title") if t_task else "Unknown task",
                "project_key": project_map.get(t_task.get("project_id"), {}).get("key") if t_task else None,
                "started_at": tmr["started_at"],
                "elapsed_seconds": elapsed,
            }

        # recent activity (last 5 entries)
        recent = []
        for e in my_entries[:5]:
            t_task = task_map.get(e["task_id"])
            recent.append({
                "task_id": e["task_id"],
                "task_title": t_task.get("title") if t_task else "Deleted task",
                "minutes": e["minutes"],
                "date": e.get("date"),
                "note": e.get("note", ""),
                "created_at": e.get("created_at"),
            })

        rows.append({
            "user_id": uid,
            "name": u.get("name"),
            "email": u.get("email"),
            "picture": u.get("picture"),
            "role": role_map.get(uid),
            "active_timer": active_timer,
            "active_count": len(active_tasks),
            "in_progress_tasks": [
                {"task_id": t["task_id"], "title": t["title"], "priority": t.get("priority"),
                 "project_key": project_map.get(t.get("project_id"), {}).get("key"),
                 "due_date": t.get("due_date")}
                for t in in_progress[:5]
            ],
            "estimate_hours": sum(t.get("estimate_hours", 0) for t in active_tasks),
            "logged_today_minutes": today_min,
            "logged_week_minutes": week_min,
            "recent_activity": recent,
        })

    # sort: those with active timer first, then by active count desc
    rows.sort(key=lambda r: (r["active_timer"] is None, -r["active_count"]))
    return {"generated_at": now_ts.isoformat(), "members": rows}


# ----------------------
# Sprint Burndown
# ----------------------
@api.get("/orgs/{org_id}/sprints/{sprint_id}/burndown")
async def sprint_burndown(org_id: str, sprint_id: str, user: dict = Depends(get_current_user)):
    await ensure_member(org_id, user["user_id"])
    sprint = await db.sprints.find_one({"sprint_id": sprint_id, "org_id": org_id}, {"_id": 0})
    if not sprint:
        raise HTTPException(status_code=404, detail="Sprint not found")
    tasks = await db.tasks.find({"org_id": org_id, "sprint_id": sprint_id}, {"_id": 0}).to_list(2000)
    total_hours = sum(t.get("estimate_hours") or 0 for t in tasks)

    if not sprint.get("start_date") or not sprint.get("end_date"):
        return {"sprint_id": sprint_id, "total_estimate_hours": total_hours, "series": []}

    start_d = datetime.fromisoformat(sprint["start_date"]).date()
    end_d = datetime.fromisoformat(sprint["end_date"]).date()
    days_total = max(1, (end_d - start_d).days)
    today = now_utc().date()

    series = []
    cursor_d = start_d
    while cursor_d <= end_d:
        idx = (cursor_d - start_d).days
        ideal = round(total_hours * (1 - idx / days_total), 2)
        # actual: total - completed_at up to end-of-day
        completed_hours = 0
        for t in tasks:
            ca = t.get("completed_at")
            if not ca:
                continue
            ca_date = datetime.fromisoformat(ca.replace("Z", "+00:00") if "Z" in ca else ca).date() if isinstance(ca, str) else ca.date()
            if ca_date <= cursor_d:
                completed_hours += t.get("estimate_hours") or 0
        actual = round(total_hours - completed_hours, 2)
        series.append({
            "date": cursor_d.isoformat(),
            "ideal": ideal,
            "actual": actual if cursor_d <= today else None,
        })
        cursor_d = cursor_d + timedelta(days=1)

    completed_count = sum(1 for t in tasks if t.get("status") == "done")
    return {
        "sprint_id": sprint_id,
        "sprint_name": sprint.get("name"),
        "start_date": sprint["start_date"],
        "end_date": sprint["end_date"],
        "total_tasks": len(tasks),
        "completed_tasks": completed_count,
        "total_estimate_hours": total_hours,
        "series": series,
    }


# ----------------------
# Task Comments
# ----------------------
import re as _re
MENTION_RE = _re.compile(r"@([\w.+-]+@[\w-]+\.[\w.-]+)")

@api.get("/orgs/{org_id}/tasks/{task_id}/comments")
async def list_comments(org_id: str, task_id: str, user: dict = Depends(get_current_user)):
    await require_task_access(org_id, task_id, user)
    comments = await db.comments.find({"org_id": org_id, "task_id": task_id}, {"_id": 0}).sort("created_at", 1).to_list(500)
    author_ids = list({c["author_id"] for c in comments})
    authors = await db.users.find({"user_id": {"$in": author_ids}}, {"_id": 0, "password_hash": 0}).to_list(500)
    amap = {u["user_id"]: u for u in authors}
    for c in comments:
        a = amap.get(c["author_id"], {})
        c["author_name"] = a.get("name") or a.get("email")
        c["author_email"] = a.get("email")
        c["author_picture"] = a.get("picture")
    return comments

@api.post("/orgs/{org_id}/tasks/{task_id}/comments")
async def create_comment(org_id: str, task_id: str, body: CommentCreate, user: dict = Depends(get_current_user)):
    task = await require_task_access(org_id, task_id, user)
    if not body.body.strip():
        raise HTTPException(status_code=422, detail="Comment cannot be empty")
    mentions = list({m.lower() for m in MENTION_RE.findall(body.body)})
    mentioned_users = []
    if mentions:
        found = await db.users.find({"email": {"$in": mentions}}, {"_id": 0, "password_hash": 0}).to_list(100)
        mentioned_users = [u["user_id"] for u in found]
    doc = {
        "comment_id": new_id("cmt"), "org_id": org_id, "task_id": task_id,
        "author_id": user["user_id"], "body": body.body,
        "mentions": mentioned_users, "mentions_emails": mentions,
        "created_at": now_utc().isoformat(),
    }
    await db.comments.insert_one(dict(doc))
    doc.pop("_id", None)
    doc["author_name"] = user.get("name") or user.get("email")
    doc["author_email"] = user.get("email")
    doc["author_picture"] = user.get("picture")
    return doc


# ----------------------
# Analytics
# ----------------------
@api.get("/orgs/{org_id}/analytics")
async def analytics(org_id: str, user: dict = Depends(get_current_user)):
    m = await ensure_member(org_id, user["user_id"])
    allowed = await _get_accessible_project_ids(org_id, user["user_id"], m["role"])
    tq = {"org_id": org_id}
    if allowed is not None:
        tq["project_id"] = {"$in": allowed}
    tasks = await db.tasks.find(tq, {"_id": 0}).to_list(5000)
    accessible_task_ids = [t["task_id"] for t in tasks]
    eq = {"org_id": org_id}
    if allowed is not None:
        eq["task_id"] = {"$in": accessible_task_ids}
    entries = await db.time_entries.find(eq, {"_id": 0}).to_list(5000)
    by_status = {"todo": 0, "in_progress": 0, "review": 0, "done": 0}
    for t in tasks:
        s = t.get("status", "todo")
        by_status[s] = by_status.get(s, 0) + 1
    # tasks completed last 7 days
    today = now_utc().date()
    last7 = [(today - timedelta(days=i)).isoformat() for i in range(6, -1, -1)]
    completed_series = []
    for d in last7:
        c = sum(1 for t in tasks if (t.get("completed_at") or "").startswith(d))
        completed_series.append({"date": d, "completed": c})
    # minutes logged last 7 days
    time_series = []
    for d in last7:
        m = sum(e["minutes"] for e in entries if e.get("date") == d)
        time_series.append({"date": d, "minutes": m})
    total_estimate = sum(t.get("estimate_hours", 0) * 60 for t in tasks)
    total_logged = sum(e["minutes"] for e in entries)
    return {
        "by_status": by_status,
        "total_tasks": len(tasks),
        "completed_tasks": by_status.get("done", 0),
        "completion_rate": (by_status.get("done", 0) / len(tasks) * 100) if tasks else 0,
        "total_logged_minutes": total_logged,
        "total_estimate_minutes": total_estimate,
        "completed_series": completed_series,
        "time_series": time_series,
    }


# ----------------------
# Email (Resend via Emergent proxy)
# ----------------------
from html import escape as _esc
from html.parser import HTMLParser as _HTMLParser
from urllib.parse import urlparse as _urlparse
import ipaddress as _ipaddr

EMAIL_BASE_URL = "https://integrations.emergentagent.com"
EMAIL_KEY = os.environ.get("EMERGENT_EMAIL_KEY")
EMAIL_FROM_NAME = os.environ.get("EMAIL_FROM_NAME", "SprintHub")

_SHORTENERS = ("bit.ly", "tinyurl.com", "t.co", "is.gd", "cutt.ly", "goo.gl", "rebrand.ly")
_CRED_ASK = ("reply with your password", "reply with the code", "send your password",
             "cvv", "send us your password", "enter your password below",
             "confirm your card number", "your full card number", "seed phrase",
             "recovery phrase", "verify your card", "social security number",
             "confirm your bank details")
_HOSTISH = _re.compile(r"\b(?:https?://)?((?:[a-z0-9-]+\.)+[a-z]{2,})", _re.I)

def _host_ok(host: str) -> bool:
    if not host or "xn--" in host:
        return False
    try:
        _ipaddr.ip_address(host)
        return False
    except ValueError:
        pass
    return not any(host == s or host.endswith("." + s) for s in _SHORTENERS)

def _same_site(shown: str, real: str) -> bool:
    return shown == real or real.endswith("." + shown) or shown.endswith("." + real)

class _EmailScan(_HTMLParser):
    def __init__(self):
        super().__init__()
        self.tags, self.urls, self.anchors = set(), [], []
        self._href, self._text = None, []
    def handle_starttag(self, tag, attrs):
        self.tags.add(tag.lower())
        self.urls += [v for k, v in attrs if k.lower() in ("href", "src") and v]
        if tag.lower() == "a":
            self._href = dict((k.lower(), v) for k, v in attrs).get("href")
            self._text = []
    def handle_data(self, data):
        if self._href is not None:
            self._text.append(data)
    def handle_endtag(self, tag):
        if tag.lower() == "a" and self._href is not None:
            self.anchors.append((self._href, "".join(self._text)))
            self._href, self._text = None, []

def _assert_safe_email(subject: str, html: str) -> None:
    scan = _EmailScan(); scan.feed(html)
    if scan.tags & {"form", "input", "textarea", "select"}:
        raise ValueError("No forms or input fields in email (G2)")
    body = f"{subject}\n{html}".lower()
    for p in _CRED_ASK:
        if p in body:
            raise ValueError(f"Email asks the recipient for credentials: {p!r} (G2)")
    for url in scan.urls:
        low = url.strip().lower()
        if low.startswith(("mailto:", "tel:", "cid:", "#")):
            continue
        if not low.startswith("https://"):
            raise ValueError(f"Email links/assets must be absolute https: {url!r} (G3)")
        host = _urlparse(low).hostname or ""
        if not _host_ok(host) or _urlparse(low).username is not None:
            raise ValueError(f"Shortened, numeric-host or credential-bearing URL: {url!r} (G3)")
    for href, text in scan.anchors:
        real = _urlparse(href.strip().lower()).hostname or ""
        if not real:
            continue
        for m in _HOSTISH.finditer(text):
            if not _same_site(m.group(1).lower(), real):
                raise ValueError(f"Anchor text {m.group(1)!r} != real link host {real!r} (G3)")

async def send_email(*, to: str, subject: str, html: str) -> Optional[str]:
    if not EMAIL_KEY:
        logger.warning("EMERGENT_EMAIL_KEY missing; skipping email send")
        return None
    _assert_safe_email(subject, html)
    payload = {"to": [to], "subject": subject, "html": html, "from_name": EMAIL_FROM_NAME}
    try:
        async with httpx.AsyncClient(timeout=30) as hc:
            r = await hc.post(f"{EMAIL_BASE_URL}/api/v1/email/send",
                              headers={"X-Email-Key": EMAIL_KEY}, json=payload)
        r.raise_for_status()
        return r.json().get("id")
    except Exception as e:
        logger.error(f"send_email failed: {e}")
        return None


# ----------------------
# Weekly Digest (Cron)
# ----------------------
async def _build_and_send_digest():
    """Build the weekly digest for each admin/owner (last 7 days hours + overdue tasks)."""
    today = now_utc().date()
    week_start = (today - timedelta(days=7)).isoformat()
    # Find all owners/admins across all orgs
    memberships = await db.memberships.find({"role": {"$in": ["owner", "admin"]}}, {"_id": 0}).to_list(2000)
    for m in memberships:
        org = await db.organizations.find_one({"org_id": m["org_id"]}, {"_id": 0})
        u = await db.users.find_one({"user_id": m["user_id"]}, {"_id": 0, "password_hash": 0})
        if not u or not u.get("email") or not org:
            continue
        # Aggregate hours by member for this org
        entries = await db.time_entries.find(
            {"org_id": m["org_id"], "date": {"$gte": week_start}}, {"_id": 0}
        ).to_list(5000)
        by_user = {}
        for e in entries:
            by_user[e["user_id"]] = by_user.get(e["user_id"], 0) + e["minutes"]
        user_ids = list(by_user.keys())
        users = await db.users.find({"user_id": {"$in": user_ids}}, {"_id": 0, "password_hash": 0}).to_list(500) if user_ids else []
        umap = {x["user_id"]: x for x in users}
        # Overdue tasks
        overdue = await db.tasks.find({
            "org_id": m["org_id"],
            "status": {"$ne": "done"},
            "due_date": {"$lt": today.isoformat()},
        }, {"_id": 0}).to_list(1000)

        rows = "".join(
            f'<tr><td style="padding:6px 12px;border-top:1px solid #E2E8F0">{_esc(umap.get(uid,{}).get("name") or umap.get(uid,{}).get("email") or "Unknown")}</td>'
            f'<td style="padding:6px 12px;border-top:1px solid #E2E8F0;text-align:right;font-family:monospace">{mins//60}h {mins%60}m</td></tr>'
            for uid, mins in sorted(by_user.items(), key=lambda x: -x[1])
        ) or '<tr><td colspan="2" style="padding:12px;color:#94A3B8">No time logged this week.</td></tr>'

        overdue_html = "".join(
            f'<li style="padding:4px 0"><strong>{_esc(t["title"])}</strong> '
            f'<span style="color:#94A3B8">— due {_esc(t.get("due_date") or "")}</span></li>'
            for t in overdue[:10]
        ) or '<li style="color:#94A3B8">No overdue tasks. Nice work.</li>'

        subject = f"Weekly digest — {org['name']}"
        html = f"""
<table role="presentation" width="100%" style="background:#F8FAFC;padding:32px 0;font-family:Arial,sans-serif">
  <tr><td align="center">
    <table role="presentation" width="600" style="background:#fff;border-radius:12px;overflow:hidden;box-shadow:0 8px 30px rgba(15,23,42,0.06)">
      <tr><td style="padding:28px 32px;background:#4F46E5;color:#fff">
        <div style="font-size:12px;letter-spacing:0.2em;text-transform:uppercase;opacity:0.8">Weekly Digest</div>
        <div style="font-size:24px;font-weight:600;margin-top:6px">{_esc(org['name'])}</div>
      </td></tr>
      <tr><td style="padding:24px 32px">
        <p style="color:#0F172A;font-size:15px">Hi {_esc(u.get('name') or u.get('email'))}, here is your team snapshot for the last 7 days.</p>
        <h3 style="color:#0F172A;font-size:16px;margin-top:24px;margin-bottom:8px">Hours logged</h3>
        <table role="presentation" width="100%" style="border-collapse:collapse;font-size:14px;color:#334155">{rows}</table>
        <h3 style="color:#0F172A;font-size:16px;margin-top:24px;margin-bottom:8px">Overdue tasks ({len(overdue)})</h3>
        <ul style="color:#334155;font-size:14px;padding-left:20px;margin:0">{overdue_html}</ul>
        <p style="font-size:12px;color:#94A3B8;margin-top:28px">Sent by {_esc(EMAIL_FROM_NAME)}. You are receiving this because you are an admin of {_esc(org['name'])}. We never ask for your password by email.</p>
      </td></tr>
    </table>
  </td></tr>
</table>
"""
        await send_email(to=u["email"], subject=subject, html=html)


@app.post("/api/cron/weekly-digest")
async def cron_weekly_digest(request: Request):
    # Cron endpoints must ack 2xx immediately; enqueue/background the actual work.
    auth = request.headers.get("Authorization", "")
    secret = os.environ.get("WEBHOOK_CRON_SECRET", "")
    if not secret or not auth.startswith("Bearer ") or auth[7:] != secret:
        raise HTTPException(status_code=401, detail="Unauthorized")
    run_id = request.headers.get("X-Webhook-Id") or f"local-{now_utc().timestamp()}"
    dup = await db.cron_runs.find_one({"run_id": run_id})
    if dup:
        return {"ok": True, "duplicate": True}
    await db.cron_runs.insert_one({"run_id": run_id, "at": now_utc().isoformat(), "name": "weekly-digest"})
    import asyncio as _asyncio
    _asyncio.create_task(_build_and_send_digest())
    return {"ok": True, "queued": True}


# ----------------------
# Startup
# ----------------------
@app.on_event("startup")
async def startup():
    await db.users.create_index("email", unique=True)
    await db.users.create_index("user_id", unique=True)
    await db.organizations.create_index("org_id", unique=True)
    await db.memberships.create_index([("org_id", 1), ("user_id", 1)], unique=True)
    await db.projects.create_index("project_id", unique=True)
    await db.tasks.create_index("task_id", unique=True)
    await db.tasks.create_index("org_id")
    await db.sprints.create_index("sprint_id", unique=True)
    await db.time_entries.create_index("entry_id", unique=True)
    await db.user_sessions.create_index("session_token", unique=True)
    # Seed admin
    admin_email = os.environ.get("ADMIN_EMAIL", "widiardhana@gmail.com")
    admin_password = os.environ.get("ADMIN_PASSWORD", "Admin@1234")
    existing = await db.users.find_one({"email": admin_email})
    if not existing:
        user_id = new_id("user")
        await db.users.insert_one({
            "user_id": user_id, "email": admin_email, "name": "Admin",
            "password_hash": hash_password(admin_password), "picture": None,
            "created_at": now_utc().isoformat()
        })
        org_id = new_id("org")
        await db.organizations.insert_one({
            "org_id": org_id, "name": "Acme Corp",
            "owner_id": user_id, "logo": None, "created_at": now_utc().isoformat()
        })
        await db.memberships.insert_one({
            "membership_id": new_id("mem"), "org_id": org_id,
            "user_id": user_id, "role": "owner", "created_at": now_utc().isoformat()
        })
        # Seed a demo project + sprint + tasks
        prj_id = new_id("prj")
        await db.projects.insert_one({
            "project_id": prj_id, "org_id": org_id,
            "name": "Web Redesign", "key": "WEB",
            "description": "Company website redesign project",
            "color": "#4F46E5", "created_by": user_id,
            "created_at": now_utc().isoformat()
        })
        await db.project_members.insert_one({
            "org_id": org_id, "project_id": prj_id, "user_id": user_id,
            "role": "lead", "added_at": now_utc().isoformat(), "added_by": user_id
        })
        spr_id = new_id("spr")
        await db.sprints.insert_one({
            "sprint_id": spr_id, "org_id": org_id, "project_id": prj_id,
            "name": "Sprint 1", "goal": "Launch new landing page",
            "start_date": now_utc().date().isoformat(),
            "end_date": (now_utc() + timedelta(days=14)).date().isoformat(),
            "status": "active", "created_at": now_utc().isoformat()
        })
        seed_tasks = [
            ("Design homepage hero", "in_progress", "high", 8),
            ("Set up analytics", "todo", "medium", 3),
            ("Write copy for pricing page", "review", "medium", 4),
            ("Fix mobile nav bug", "done", "high", 2),
            ("Daily standup", "todo", "low", 0.5),
        ]
        for i, (title, status, prio, est) in enumerate(seed_tasks):
            await db.tasks.insert_one({
                "task_id": new_id("tsk"), "org_id": org_id, "project_id": prj_id,
                "title": title, "description": "", "status": status,
                "priority": prio, "type": "task", "assignee_id": user_id,
                "sprint_id": spr_id,
                "start_date": now_utc().date().isoformat(),
                "due_date": (now_utc() + timedelta(days=i + 2)).date().isoformat(),
                "estimate_hours": est, "logged_minutes": 60 if status == "done" else 0,
                "created_by": user_id,
                "created_at": now_utc().isoformat(),
                "updated_at": now_utc().isoformat(),
                "completed_at": now_utc().isoformat() if status == "done" else None
            })
    else:
        # update password to match .env
        if not verify_password(admin_password, existing["password_hash"]):
            await db.users.update_one({"email": admin_email}, {"$set": {"password_hash": hash_password(admin_password)}})
    # Backfill: ensure every project has its creator as a project_member (idempotent)
    all_projects = await db.projects.find({}, {"_id": 0}).to_list(5000)
    for p in all_projects:
        exists = await db.project_members.find_one({"org_id": p["org_id"], "project_id": p["project_id"], "user_id": p.get("created_by")})
        if not exists and p.get("created_by"):
            await db.project_members.insert_one({
                "org_id": p["org_id"], "project_id": p["project_id"], "user_id": p["created_by"],
                "role": "lead", "added_at": now_utc().isoformat(), "added_by": p["created_by"]
            })
    logger.info("Startup complete")

@app.on_event("shutdown")
async def shutdown():
    client.close()

app.include_router(api)

app.add_middleware(
    CORSMiddleware,
    allow_credentials=True,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
    expose_headers=["*"],
)
