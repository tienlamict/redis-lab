import { useState } from "react";
import { useTopology } from "../hooks/useTopology";
import { LabLayout } from "../components/LabLayout";
import { TopologyView } from "../components/TopologyView";

export function StandalonePage() {
  const { data, refresh } = useTopology("standalone");
  const [selected, setSelected] = useState<string | null>(null);

  const pods = data?.pods ?? [];
  const masterPod = data?.master?.pod || pods.find((p) => p.name.includes("master"))?.name;
  const masterPods = masterPod ? [masterPod] : [];
  const selPod = pods.find((p) => p.name === selected);

  return (
    <LabLayout
      lab="standalone"
      pods={pods}
      masterPods={masterPods}
      topology={
        <TopologyView lab="standalone" data={data} onNodeClick={setSelected} selected={selected} />
      }
      detailPanel={
        selPod ? (
          <div className="space-y-1">
            <div className="text-sm font-semibold text-slate-100">{selPod.name}</div>
            <div className="text-slate-400">node: <span className="text-slate-200">{selPod.node}</span></div>
            <div className="text-slate-400">ip: <span className="text-slate-200">{selPod.ip || "(pending)"}</span></div>
            <div className="text-slate-400">status: <span className="text-slate-200">{selPod.status}</span></div>
            <div className="text-slate-400">ready: <span className="text-slate-200">{String(selPod.ready)}</span></div>
            <div className="text-slate-400">role: <span className="text-slate-200">{masterPod === selPod.name ? "master" : "replica"}</span></div>
          </div>
        ) : (
          <div className="text-slate-500">Click a node to inspect.</div>
        )
      }
      extra={
        <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-amber-200">
          <strong>Replication is not HA.</strong> Killing the master breaks
          writes — there is no auto-promote. That's the reason Sentinel exists.
        </div>
      }
      onSseTick={refresh}
    />
  );
}
