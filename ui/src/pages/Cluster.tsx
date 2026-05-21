import { useState } from "react";
import { useTopology } from "../hooks/useTopology";
import { LabLayout } from "../components/LabLayout";
import { TopologyView } from "../components/TopologyView";
import { SlotMap } from "../components/SlotMap";

export function ClusterPage() {
  const { data, refresh } = useTopology("cluster");
  const [selected, setSelected] = useState<string | null>(null);
  const shards = data?.shards ?? [];
  const masterPods = shards.map((s) => s.masterPod).filter(Boolean);
  const pods = data?.pods ?? [];
  const selPod = pods.find((p) => p.name === selected);
  const shardOf = (pod: string) => {
    for (const sh of shards) {
      if (sh.masterPod === pod) return { role: "master", slots: sh.slotRanges, masterPod: sh.masterPod };
      if (sh.replicas.some((r) => r.pod === pod))
        return { role: "replica", slots: sh.slotRanges, masterPod: sh.masterPod };
    }
    return null;
  };
  const info = selPod ? shardOf(selPod.name) : null;

  return (
    <LabLayout
      lab="cluster"
      pods={pods}
      masterPods={masterPods}
      topology={
        <TopologyView lab="cluster" data={data} onNodeClick={setSelected} selected={selected} />
      }
      detailPanel={
        selPod ? (
          <div className="space-y-1">
            <div className="text-sm font-semibold text-slate-100">{selPod.name}</div>
            <div className="text-slate-400">node: <span className="text-slate-200">{selPod.node}</span></div>
            <div className="text-slate-400">ip: <span className="text-slate-200">{selPod.ip || "(pending)"}</span></div>
            {info && (
              <>
                <div className="text-slate-400">role: <span className="text-slate-200">{info.role}</span></div>
                <div className="text-slate-400">slots: <span className="text-slate-200">{info.slots.map((r) => `${r[0]}-${r[1]}`).join(",")}</span></div>
                <div className="text-slate-400">in shard: <span className="text-slate-200">{info.masterPod}</span></div>
              </>
            )}
          </div>
        ) : (
          <div className="text-slate-500">Click a node to inspect.</div>
        )
      }
      extra={
        <div className="mt-2 space-y-2">
          <SlotMap shards={shards} />
          <div className="rounded border border-cyan-500/30 bg-cyan-500/5 px-3 py-2 text-xs text-cyan-200">
            Try <span className="font-mono">SET user:{"{42}"}:profile alice</span>{" "}
            then <span className="font-mono">SET user:{"{42}"}:posts 100</span>{" "}
            — same hash tag → same slot. Compare with{" "}
            <span className="font-mono">MGET a b</span> on unrelated keys for a
            CROSSSLOT error.
          </div>
        </div>
      }
      onSseTick={refresh}
    />
  );
}
