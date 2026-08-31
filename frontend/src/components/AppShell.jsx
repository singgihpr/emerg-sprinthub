import React, { useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "@/context/AuthContext";
import { useOrg } from "@/context/OrgContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent,
  DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  House, ListChecks, FolderOpen, Users, ChartBar, CaretUpDown, Plus,
  SignOut, Buildings, Rocket, Pulse, Gear, UserCircle,
} from "@phosphor-icons/react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import TaskTimer from "@/components/TaskTimer";
import OrgSettingsDialog from "@/components/OrgSettingsDialog";

const NAV = [
  { to: "/dashboard", label: "Dashboard", icon: House },
  { to: "/tasks", label: "Tasks", icon: ListChecks },
  { to: "/projects", label: "Projects", icon: FolderOpen },
  { to: "/sprints", label: "Sprints", icon: Rocket },
  { to: "/team", label: "Team Activity", icon: Pulse, roles: ["owner", "admin", "manager"] },
  { to: "/members", label: "Members", icon: Users },
  { to: "/analytics", label: "Analytics", icon: ChartBar },
];

export default function AppShell() {
  const { user, logout } = useAuth();
  const { orgs, currentOrg, switchOrg, createOrg } = useOrg();
  const nav = useNavigate();
  const [orgMenuOpen, setOrgMenuOpen] = useState(false);
  const [newOrgOpen, setNewOrgOpen] = useState(false);
  const [orgSettingsOpen, setOrgSettingsOpen] = useState(false);
  const [newOrgName, setNewOrgName] = useState("");

  const handleLogout = async () => { await logout(); nav("/login"); };
  const initials = (user?.name || user?.email || "?").slice(0, 2).toUpperCase();

  return (
    <div className="min-h-screen flex bg-slate-50">
      {/* Sidebar */}
      <aside className="w-64 bg-white border-r border-slate-200 flex flex-col shrink-0">
        <div className="p-4 border-b border-slate-200">
          <DropdownMenu open={orgMenuOpen} onOpenChange={setOrgMenuOpen}>
            <DropdownMenuTrigger asChild>
              <button
                data-testid="org-switcher-btn"
                className="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg hover:bg-slate-50 transition-colors group"
              >
                <div className="w-8 h-8 rounded-md bg-indigo-600 text-white flex items-center justify-center text-sm font-semibold shrink-0 overflow-hidden">
                  {currentOrg?.logo ? (
                    <img src={currentOrg.logo} alt="" className="w-full h-full object-cover" />
                  ) : (
                    (currentOrg?.name || "?").slice(0, 1).toUpperCase()
                  )}
                </div>
                <div className="flex-1 text-left min-w-0">
                  <div className="text-sm font-semibold text-slate-900 truncate">{currentOrg?.name || "Select workspace"}</div>
                  <div className="text-xs text-slate-500 capitalize">{currentOrg?.role || "—"}</div>
                </div>
                <CaretUpDown size={16} className="text-slate-400 shrink-0" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-64">
              <DropdownMenuLabel className="text-xs uppercase tracking-wider text-slate-500">Workspaces</DropdownMenuLabel>
              {orgs.map((o) => (
                <DropdownMenuItem key={o.org_id} onClick={() => switchOrg(o)} data-testid={`org-item-${o.org_id}`}>
                  <Buildings size={16} className="mr-2 text-slate-500" />
                  <span className="flex-1">{o.name}</span>
                  {o.org_id === currentOrg?.org_id && <span className="text-xs text-indigo-600">●</span>}
                </DropdownMenuItem>
              ))}
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => { setOrgMenuOpen(false); setOrgSettingsOpen(true); }} data-testid="org-settings-menu">
                <Gear size={16} className="mr-2" /> Workspace settings
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => { setOrgMenuOpen(false); setNewOrgOpen(true); }} data-testid="create-org-menu">
                <Plus size={16} className="mr-2" /> Create workspace
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        <nav className="flex-1 p-3 space-y-1">
          {NAV.filter((it) => !it.roles || it.roles.includes(currentOrg?.role)).map((it) => (
            <NavLink
              key={it.to} to={it.to}
              data-testid={`nav-${it.to.slice(1)}`}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                  isActive ? "bg-indigo-50 text-indigo-700 font-medium" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"
                }`
              }
            >
              <it.icon size={18} weight="duotone" />
              {it.label}
            </NavLink>
          ))}
        </nav>

        <div className="p-3 border-t border-slate-200">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button data-testid="user-menu-btn" className="w-full flex items-center gap-3 px-2 py-2 rounded-md hover:bg-slate-50">
                <Avatar className="h-8 w-8">
                  <AvatarImage src={user?.picture} />
                  <AvatarFallback className="bg-slate-200 text-slate-700 text-xs">{initials}</AvatarFallback>
                </Avatar>
                <div className="text-left flex-1 min-w-0">
                  <div className="text-sm font-medium text-slate-900 truncate">{user?.name || "User"}</div>
                  <div className="text-xs text-slate-500 truncate">{user?.email}</div>
                </div>
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuLabel>Account</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => nav("/profile")} data-testid="profile-menu">
                <UserCircle size={16} className="mr-2" /> Profile
              </DropdownMenuItem>
              <DropdownMenuItem onClick={handleLogout} data-testid="logout-btn">
                <SignOut size={16} className="mr-2" /> Sign out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </aside>

      {/* Main */}
      <div className="flex-1 flex flex-col min-w-0">
        <header className="h-16 bg-white/80 backdrop-blur-xl border-b border-slate-200 flex items-center justify-end px-6 sticky top-0 z-10">
          <TaskTimer />
        </header>
        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>

      <Dialog open={newOrgOpen} onOpenChange={setNewOrgOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>Create workspace</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <Input value={newOrgName} onChange={(e) => setNewOrgName(e.target.value)} placeholder="Workspace name" data-testid="new-org-name" />
            <Button
              className="w-full bg-indigo-600 hover:bg-indigo-700"
              onClick={async () => { if (!newOrgName.trim()) return; await createOrg(newOrgName.trim()); setNewOrgName(""); setNewOrgOpen(false); }}
              data-testid="submit-new-org"
            >Create</Button>
          </div>
        </DialogContent>
      </Dialog>

      <OrgSettingsDialog open={orgSettingsOpen} onOpenChange={setOrgSettingsOpen} />
    </div>
  );
}
