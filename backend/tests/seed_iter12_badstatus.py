"""Force an invalid status onto a throwaway project (or restore) to test UI fallback.
Usage: python seed_iter12_badstatus.py set|unset
"""
import asyncio
import sys

from dotenv import dotenv_values
from motor.motor_asyncio import AsyncIOMotorClient

env = dotenv_values("/app/backend/.env")
NAME = "TEST_QA i12 badstatus UI"


async def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "set"
    db = AsyncIOMotorClient(env["MONGO_URL"])[env["DB_NAME"]]
    if mode == "set":
        org = await db.organizations.find_one({"name": {"$regex": "^Acme"}}, {"_id": 0})
        existing = await db.projects.find_one({"name": NAME})
        if not existing:
            pid = "prj_i12badstatus"
            await db.projects.insert_one({
                "project_id": pid, "org_id": org["org_id"], "name": NAME, "key": "I12BS",
                "description": "throwaway bad status", "color": "#4F46E5", "status": "garbage",
                "created_by": "seed", "created_at": "2026-07-01T00:00:00+00:00",
            })
        else:
            pid = existing["project_id"]
            await db.projects.update_one({"project_id": pid}, {"$set": {"status": "garbage"}})
        print("seeded", pid, "org", org["org_id"])
    else:
        print("deleted:", (await db.projects.delete_many({"name": NAME})).deleted_count)


asyncio.run(main())
