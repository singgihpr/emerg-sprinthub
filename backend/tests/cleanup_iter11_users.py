"""Remove throwaway test_qa_mgr_* users + their memberships created by iteration-11 RBAC tests."""
import asyncio
from motor.motor_asyncio import AsyncIOMotorClient
from dotenv import dotenv_values

env = dotenv_values("/app/backend/.env")


async def main():
    db = AsyncIOMotorClient(env["MONGO_URL"])[env["DB_NAME"]]
    ids = [u["user_id"] async for u in db.users.find({"email": {"$regex": "test_qa_mgr_"}}, {"_id": 0, "user_id": 1})]
    print("removing users:", ids)
    if ids:
        print("memberships:", (await db.memberships.delete_many({"user_id": {"$in": ids}})).deleted_count)
        print("project_members:", (await db.project_members.delete_many({"user_id": {"$in": ids}})).deleted_count)
        print("users:", (await db.users.delete_many({"user_id": {"$in": ids}})).deleted_count)


asyncio.run(main())
