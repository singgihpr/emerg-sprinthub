import React, { createContext, useContext, useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";

const OrgContext = createContext(null);
export const useOrg = () => useContext(OrgContext);

export function OrgProvider({ children }) {
  const { user } = useAuth();
  const [orgs, setOrgs] = useState([]);
  const [currentOrg, setCurrentOrg] = useState(null);

  const loadOrgs = useCallback(async () => {
    const { data } = await api.get("/orgs");
    setOrgs(data);
    const savedId = localStorage.getItem("current_org");
    const found = data.find((o) => o.org_id === savedId) || data[0];
    setCurrentOrg(found || null);
    if (found) localStorage.setItem("current_org", found.org_id);
  }, []);

  useEffect(() => {
    if (user && user.user_id) loadOrgs();
  }, [user, loadOrgs]);

  const switchOrg = (org) => {
    setCurrentOrg(org);
    localStorage.setItem("current_org", org.org_id);
  };

  const createOrg = async (name) => {
    const { data } = await api.post("/orgs", { name });
    setOrgs((p) => [...p, data]);
    switchOrg(data);
    return data;
  };

  return (
    <OrgContext.Provider value={{ orgs, currentOrg, switchOrg, createOrg, reload: loadOrgs }}>
      {children}
    </OrgContext.Provider>
  );
}
