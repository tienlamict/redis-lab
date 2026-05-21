// Scrolling, color-coded event log capped at 200 lines.
import { useEffect, useRef } from "react";
import type { SseEvent } from "../types";

const COLORS: Record<string, string> = {
  pod_added: "text-emerald-300",
  pod_modified: "text-slate-300",
  pod_deleted: "text-rose-300",
  role_changed: "text-amber-300",
  sentinel_event: "text-violet-300",
  cluster_event: "text-cyan-300",
  hello: "text-slate-500",
  pod_event: "text-slate-300",
};

export function EventLog({ events }: { events: SseEvent[] }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight });
  }, [events]);
  return (
    <div className="flex h-full flex-col rounded-lg border border-slate-800 bg-black/40">
      <div className="border-b border-slate-800 px-3 py-1.5 text-xs text-slate-400">
        Event log
      </div>
      <div
        ref={ref}
        className="flex-1 overflow-y-auto px-3 py-2 font-mono text-xs leading-relaxed"
      >
        {events.length === 0 && (
          <div className="text-slate-500">Waiting for events…</div>
        )}
        {events.map((e, i) => {
          const ts = new Date(e.time).toLocaleTimeString();
          let detail = "";
          if (e.payload) {
            if ("pod" in e.payload && e.payload.pod) {
              const p = e.payload.pod as { name?: string; status?: string };
              detail = `${p.name ?? ""} (${p.status ?? ""})`;
            } else if ("channel" in e.payload) {
              const ch = String(e.payload.channel);
              const pl = String(e.payload.payload ?? "");
              detail = `${ch} ${pl}`;
            } else {
              detail = JSON.stringify(e.payload);
            }
          }
          return (
            <div key={i} className={COLORS[e.type] ?? "text-slate-300"}>
              <span className="text-slate-500">{ts}</span>{" "}
              <span className="opacity-90">[{e.type}]</span> {detail}
            </div>
          );
        })}
      </div>
    </div>
  );
}
