// Shared layout for every lab page. Top: topology. Middle row: console +
// fault. Bottom: event log. Hosts SSE subscription + topology refresh.
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { Lab, PodSummary, SseEvent } from "../types";
import { useSSE } from "../hooks/useSSE";
import { EventLog } from "./EventLog";
import { FaultPanel } from "./FaultPanel";
import { RedisConsole } from "./RedisConsole";

export function LabLayout(props: {
  lab: Lab;
  pods: PodSummary[];
  masterPod?: string;
  topology: ReactNode;
  extra?: ReactNode;
  onSseTick?: () => void;
}) {
  const [events, setEvents] = useState<SseEvent[]>([]);
  const onTick = useRef(props.onSseTick);
  onTick.current = props.onSseTick;

  useSSE(props.lab, (e) => {
    setEvents((prev) => [...prev.slice(-199), e]);
    if (e.type !== "hello") onTick.current?.();
  });

  useEffect(() => {
    setEvents([]);
  }, [props.lab]);

  return (
    <div className="flex h-[calc(100vh-49px)] flex-col gap-3 p-3">
      <section className="flex-shrink-0 rounded-lg border border-slate-800 bg-slate-950/30 p-3">
        {props.topology}
        {props.extra}
      </section>
      <section className="grid min-h-[260px] flex-1 grid-cols-1 gap-3 md:grid-cols-3">
        <div className="md:col-span-2 min-h-0">
          <RedisConsole lab={props.lab} />
        </div>
        <div className="min-h-0">
          <FaultPanel lab={props.lab} pods={props.pods} masterPod={props.masterPod} />
        </div>
      </section>
      <section className="h-44 flex-shrink-0">
        <EventLog events={events} />
      </section>
    </div>
  );
}
