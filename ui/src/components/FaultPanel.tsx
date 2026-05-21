// Renders kill buttons grouped by master/replica role + elapsed timer since
// last kill. Pulls pod list from a topology snapshot.
import { useEffect, useState } from "react";
import { api } from "../api";
import type { Lab, PodSummary } from "../types";

export function FaultPanel(props: {
  lab: Lab;
  pods: PodSummary[];
  // masterPods: explicit set of pods to render with the "master" (red) accent.
  // Cluster mode has 3 masters; standalone & sentinel have 1. The pages
  // compute this from their topology snapshot.
  masterPods?: string[];
  // failoverActive: while true, an "elapsed" timer ticks and shows how long
  // we've been waiting for the failover event. The page clears this when it
  // observes role_changed / +switch-master / pod_added (new master ready).
  failoverActive?: boolean;
  failoverStartedAt?: number;
  failoverDoneLabel?: string;
}) {
  const [last, setLast] = useState<{ pod: string; at: number } | null>(null);
  const [now, setNow] = useState(Date.now());
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 500);
    return () => clearInterval(id);
  }, []);

  const onKill = async (target: string) => {
    if (!confirm(`Kill pod ${target}? This deletes the pod and lets the StatefulSet recreate it.`)) return;
    setBusy(target);
    try {
      await api.killPod(props.lab, target);
      setLast({ pod: target, at: Date.now() });
    } catch (e) {
      alert("kill failed: " + e);
    } finally {
      setBusy(null);
    }
  };

  const masterSet = new Set(props.masterPods ?? []);
  const isMaster = (p: PodSummary) =>
    masterSet.has(p.name) || (props.lab === "standalone" && p.name.includes("master") && masterSet.size === 0);

  return (
    <div className="flex h-full flex-col rounded-lg border border-slate-800 bg-slate-950/50 p-3">
      <div className="mb-2 text-sm font-semibold text-rose-300">Fault injection</div>
      <div className="flex flex-col gap-2 overflow-y-auto">
        {props.pods.length === 0 && (
          <div className="text-sm text-slate-500">No pods yet…</div>
        )}
        {props.pods.map((p) => {
          const master = isMaster(p);
          return (
            <button
              key={p.name}
              disabled={busy === p.name}
              onClick={() => onKill(p.name)}
              className={`flex items-center justify-between rounded-md border px-3 py-1.5 text-left text-sm font-mono transition disabled:opacity-50 ${
                master
                  ? "border-rose-500/60 bg-rose-500/10 text-rose-100 hover:bg-rose-500/20"
                  : "border-amber-500/40 bg-amber-500/10 text-amber-100 hover:bg-amber-500/20"
              }`}
            >
              <span>{p.name}</span>
              <span className="text-xs opacity-75">
                {master ? "kill master" : "kill"}
              </span>
            </button>
          );
        })}
      </div>
      {(last || props.failoverActive) && (
        <div className="mt-3 border-t border-slate-800 pt-2 text-xs">
          {last && (
            <div className="text-slate-400">
              Last kill: <span className="text-slate-200">{last.pod}</span> ·{" "}
              {Math.floor((now - last.at) / 1000)}s ago
            </div>
          )}
          {props.failoverActive && props.failoverStartedAt && (
            <div className="text-amber-300">
              Failover: elapsed {Math.floor((now - props.failoverStartedAt) / 1000)}s
            </div>
          )}
          {!props.failoverActive && props.failoverDoneLabel && (
            <div className="text-emerald-300">
              Failover: {props.failoverDoneLabel}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
