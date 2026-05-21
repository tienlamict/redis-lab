import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { useState } from "react";
import { api } from "../api";
import type { Lab } from "../types";

const tabs: { to: string; label: string; lab: Lab }[] = [
  { to: "/standalone", label: "Standalone", lab: "standalone" },
  { to: "/sentinel", label: "Sentinel", lab: "sentinel" },
  { to: "/cluster", label: "Cluster", lab: "cluster" },
];

export function Navbar() {
  const loc = useLocation();
  const nav = useNavigate();
  const [resetting, setResetting] = useState(false);
  const currentLab = (loc.pathname.split("/")[1] as Lab) || "standalone";

  const onReset = async () => {
    if (!confirm(`Reset lab '${currentLab}'? This uninstalls + reinstalls the chart (~1-2 min).`)) return;
    setResetting(true);
    try {
      await api.resetLab(currentLab);
    } catch (e) {
      alert("reset failed: " + e);
    } finally {
      setResetting(false);
      // Force topology refresh by navigating to same route.
      nav(loc.pathname);
    }
  };

  return (
    <header className="flex items-center justify-between border-b border-slate-800 bg-slate-950 px-4 py-2">
      <div className="flex items-center gap-2">
        <span className="text-lg font-semibold text-rose-400">Redis Lab</span>
        <nav className="ml-4 flex gap-1">
          {tabs.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              className={({ isActive }) =>
                `rounded-md px-3 py-1.5 text-sm font-medium transition ${
                  isActive
                    ? "bg-slate-800 text-white"
                    : "text-slate-400 hover:text-slate-100"
                }`
              }
            >
              {t.label}
            </NavLink>
          ))}
        </nav>
      </div>
      <button
        onClick={onReset}
        disabled={resetting}
        className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-1.5 text-sm text-amber-300 hover:bg-amber-500/20 disabled:opacity-50"
      >
        {resetting ? "Resetting…" : `Reset ${currentLab}`}
      </button>
    </header>
  );
}
