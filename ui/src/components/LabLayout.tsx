// Shared layout for every lab page. Top: topology. Middle row: console +
// fault. Bottom: event log. Hosts SSE subscription + topology refresh +
// failover timer state (driven by role_changed / sentinel switch events).
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { Lab, PodSummary, SseEvent } from "../types";
import { useSSE } from "../hooks/useSSE";
import { EventLog } from "./EventLog";
import { FaultPanel } from "./FaultPanel";
import { RedisConsole } from "./RedisConsole";

export function LabLayout(props: {
  lab: Lab;
  pods: PodSummary[];
  masterPods?: string[];
  topology: ReactNode;
  extra?: ReactNode;
  onSseTick?: () => void;
  detailPanel?: ReactNode;
}) {
  const [events, setEvents] = useState<SseEvent[]>([]);
  const [failover, setFailover] = useState<{ startedAt: number; doneLabel?: string } | null>(null);
  const onTick = useRef(props.onSseTick);
  onTick.current = props.onSseTick;

  useSSE(props.lab, (e) => {
    setEvents((prev) => [...prev.slice(-199), e]);
    if (e.type === "hello") return;
    onTick.current?.();

    // Start failover timer the moment a master pod is deleted (kill).
    if (e.type === "pod_deleted" && e.payload) {
      const p = (e.payload as { pod?: { name?: string } }).pod;
      if (p?.name) setFailover({ startedAt: Date.now() });
    }
    // Stop timer when sentinel switches master OR the topology says we have a
    // healthy master again (best-effort: role_changed event from sentinel).
    if (e.type === "role_changed" || e.type === "sentinel_event") {
      const ch = (e.payload as { channel?: string })?.channel ?? "";
      if (ch.startsWith("+switch-master") || e.type === "role_changed") {
        setFailover((prev) =>
          prev ? { ...prev, doneLabel: `completed in ${Math.floor((Date.now() - prev.startedAt) / 1000)}s` } : prev,
        );
        setTimeout(() => setFailover(null), 8000);
      }
    }
  });

  useEffect(() => {
    setEvents([]);
    setFailover(null);
  }, [props.lab]);

  return (
    <div className="flex h-[calc(100vh-49px)] flex-col gap-3 p-3">
      <section className="flex-shrink-0 rounded-lg border border-slate-800 bg-slate-950/30 p-3">
        <div className={`grid gap-3 ${props.detailPanel ? "md:grid-cols-[1fr_240px]" : ""}`}>
          <div>{props.topology}</div>
          {props.detailPanel && (
            <aside className="rounded border border-slate-800 bg-slate-950/50 p-3 text-xs">
              {props.detailPanel}
            </aside>
          )}
        </div>
        {props.extra}
      </section>
      <section className="grid min-h-[260px] flex-1 grid-cols-1 gap-3 md:grid-cols-3">
        <div className="md:col-span-2 min-h-0">
          <RedisConsole lab={props.lab} />
        </div>
        <div className="min-h-0">
          <FaultPanel
            lab={props.lab}
            pods={props.pods}
            masterPods={props.masterPods}
            failoverActive={!!failover && !failover.doneLabel}
            failoverStartedAt={failover?.startedAt}
            failoverDoneLabel={failover?.doneLabel}
          />
        </div>
      </section>
      <section className="h-44 flex-shrink-0">
        <EventLog events={events} />
      </section>
    </div>
  );
}
