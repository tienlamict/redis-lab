import { useState } from "react";
import { useTopology } from "../hooks/useTopology";
import { LabLayout } from "../components/LabLayout";
import { TopologyView } from "../components/TopologyView";

export function SentinelPage() {
  const { data, refresh } = useTopology("sentinel");
  const [selected, setSelected] = useState<string | null>(null);
  const masterPods = data?.currentMaster ? [data.currentMaster] : [];
  const pods = data?.pods ?? [];
  const selPod = pods.find((p) => p.name === selected);

  return (
    <LabLayout
      lab="sentinel"
      pods={pods}
      masterPods={masterPods}
      topology={
        <TopologyView lab="sentinel" data={data} onNodeClick={setSelected} selected={selected} />
      }
      detailPanel={
        selPod ? (
          <div className="space-y-1">
            <div className="text-sm font-semibold text-slate-100">{selPod.name}</div>
            <div className="text-slate-400">node: <span className="text-slate-200">{selPod.node}</span></div>
            <div className="text-slate-400">ip: <span className="text-slate-200">{selPod.ip || "(pending)"}</span></div>
            <div className="text-slate-400">status: <span className="text-slate-200">{selPod.status}</span></div>
            <div className="text-slate-400">role: <span className="text-slate-200">{data?.currentMaster === selPod.name ? "master" : "replica"}</span></div>
            <div className="text-slate-400">hosts sentinel: <span className="text-slate-200">{data?.sentinels?.some((s) => s.pod === selPod.name) ? "yes" : "no"}</span></div>
          </div>
        ) : (
          <div className="text-slate-500">Click a node to inspect.</div>
        )
      }
      extra={
        <div className="mt-2 rounded border border-violet-500/30 bg-violet-500/5 px-3 py-2 text-xs text-violet-200">
          Bitnami runs redis + sentinel in the same pod. Killing a pod kills
          both. Watch the event log for{" "}
          <span className="font-mono">+sdown → +odown → +switch-master</span>{" "}
          when you kill the current master.
        </div>
      }
      onSseTick={refresh}
    />
  );
}
