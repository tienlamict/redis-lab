// Reusable colored rounded-rect representing one redis (or sentinel) node.
// Used as a JSX wrapper for layout cards (the SVG nodes live inline in
// TopologyView for animation control).
import type { ReactNode } from "react";

export type NodeStatus = "ok" | "pending" | "sdown" | "odown" | "disconnected" | "unknown" | "error";

const palette: Record<string, string> = {
  master: "border-emerald-400/60 bg-emerald-500/15 text-emerald-100",
  slave: "border-sky-400/60 bg-sky-500/15 text-sky-100",
  replica: "border-sky-400/60 bg-sky-500/15 text-sky-100",
  sentinel: "border-violet-400/60 bg-violet-500/15 text-violet-100",
};
const statusPalette: Partial<Record<NodeStatus, string>> = {
  sdown: "border-amber-400/60 bg-amber-500/15 text-amber-100",
  odown: "border-rose-400/70 bg-rose-500/20 text-rose-100",
  error: "border-rose-400/70 bg-rose-500/20 text-rose-100",
  pending: "border-amber-400/60 bg-amber-500/15 text-amber-100",
  disconnected: "border-rose-400/70 bg-rose-500/20 text-rose-100",
  unknown: "border-slate-500 bg-slate-700/30 text-slate-200",
};

export function NodeCard(props: {
  title: string;
  subtitle?: string;
  role?: string;
  status?: NodeStatus;
  children?: ReactNode;
}) {
  const cls =
    statusPalette[props.status ?? "unknown"] ??
    palette[props.role ?? ""] ??
    "border-slate-500 bg-slate-700/30 text-slate-200";
  return (
    <div className={`rounded-lg border px-3 py-2 ${cls}`}>
      <div className="font-mono text-sm font-semibold">{props.title}</div>
      {props.subtitle && <div className="text-xs opacity-80">{props.subtitle}</div>}
      {props.children}
    </div>
  );
}
