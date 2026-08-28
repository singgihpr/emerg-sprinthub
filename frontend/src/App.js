import React from "react";
import "@/App.css";
import { BrowserRouter, Routes, Route, Navigate, useLocation } from "react-router-dom";
import { AuthProvider, useAuth } from "@/context/AuthContext";
import { OrgProvider } from "@/context/OrgContext";
import { Toaster } from "@/components/ui/sonner";
import Login from "@/pages/Login";
import AuthCallback from "@/pages/AuthCallback";
import AppShell from "@/components/AppShell";
import Dashboard from "@/pages/Dashboard";
import Tasks from "@/pages/Tasks";
import Projects from "@/pages/Projects";
import Sprints from "@/pages/Sprints";
import Members from "@/pages/Members";
import Analytics from "@/pages/Analytics";

function ProtectedRoute({ children }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="min-h-screen flex items-center justify-center text-slate-500">Loading…</div>;
  if (!user || !user.user_id) return <Navigate to="/login" replace />;
  return children;
}

function AppRoutes() {
  const location = useLocation();
  if (location.hash?.includes("session_id=")) return <AuthCallback />;
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <OrgProvider>
              <AppShell />
            </OrgProvider>
          </ProtectedRoute>
        }
      >
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<Dashboard />} />
        <Route path="tasks" element={<Tasks />} />
        <Route path="projects" element={<Projects />} />
        <Route path="sprints" element={<Sprints />} />
        <Route path="members" element={<Members />} />
        <Route path="analytics" element={<Analytics />} />
      </Route>
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  );
}

export default function App() {
  return (
    <div className="App">
      <BrowserRouter>
        <AuthProvider>
          <AppRoutes />
          <Toaster />
        </AuthProvider>
      </BrowserRouter>
    </div>
  );
}
