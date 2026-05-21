// useSSE: subscribes to /sse/:lab via EventSource and feeds parsed events to a
// callback. Reconnects with exponential backoff (1s, 2s, 4s, ..., max 30s)
// when the stream drops.
import { useEffect, useRef } from "react";
import type { Lab, SseEvent } from "../types";

export function useSSE(lab: Lab, onEvent: (e: SseEvent) => void) {
  const cbRef = useRef(onEvent);
  cbRef.current = onEvent;

  useEffect(() => {
    let es: EventSource | null = null;
    let backoff = 1000;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let stopped = false;

    const connect = () => {
      if (stopped) return;
      es = new EventSource(`/sse/${lab}`);
      // Generic message (rare — backend uses named events).
      es.onmessage = (m) => {
        try {
          cbRef.current(JSON.parse(m.data) as SseEvent);
        } catch {
          /* ignore */
        }
      };
      // Each event type registers an individual listener.
      const types: SseEvent["type"][] = [
        "hello",
        "pod_added",
        "pod_modified",
        "pod_deleted",
        "role_changed",
        "sentinel_event",
        "cluster_event",
        "pod_event",
      ];
      for (const t of types) {
        es.addEventListener(t, (ev) => {
          try {
            const data = (ev as MessageEvent).data;
            const parsed = JSON.parse(data) as Partial<SseEvent>;
            cbRef.current({
              lab,
              type: t,
              time: parsed.time ?? new Date().toISOString(),
              payload: parsed.payload,
            });
          } catch {
            /* ignore */
          }
        });
      }
      es.onopen = () => {
        backoff = 1000;
      };
      es.onerror = () => {
        es?.close();
        es = null;
        if (stopped) return;
        timer = setTimeout(connect, backoff);
        backoff = Math.min(backoff * 2, 30_000);
      };
    };

    connect();
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
      es?.close();
    };
  }, [lab]);
}
