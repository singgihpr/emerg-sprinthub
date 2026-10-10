import axios from "axios";

const BACKEND_URL = process.env.REACT_APP_BACKEND_URL;
export const API = `${BACKEND_URL}/api`;

export const api = axios.create({
  baseURL: API,
  withCredentials: true,
});

api.interceptors.request.use((config) => {
  const t = localStorage.getItem("access_token");
  if (t) config.headers.Authorization = `Bearer ${t}`;
  return config;
});

// 401 → try cookie-based refresh once, then retry the original request.
let refreshing = null;
api.interceptors.response.use(
  (res) => res,
  async (err) => {
    const config = err.config;
    if (
      err.response?.status === 401 &&
      config &&
      !config._retried &&
      !config.url?.startsWith("/auth/")
    ) {
      config._retried = true;
      if (!refreshing) {
        refreshing = axios
          .post(`${BACKEND_URL}/auth/refresh`, {}, { withCredentials: true })
          .finally(() => {
            refreshing = null;
          });
      }
      try {
        const { data } = await refreshing;
        if (data.token) localStorage.setItem("access_token", data.token);
        return api(config);
      } catch {
        localStorage.removeItem("access_token");
      }
    }
    return Promise.reject(err);
  }
);

export function formatApiErrorDetail(detail) {
  if (detail == null) return "Something went wrong. Please try again.";
  if (typeof detail === "string") return detail;
  if (Array.isArray(detail))
    return detail
      .map((e) => (e && typeof e.msg === "string" ? e.msg : JSON.stringify(e)))
      .filter(Boolean)
      .join(" ");
  if (detail && typeof detail.msg === "string") return detail.msg;
  return String(detail);
}
