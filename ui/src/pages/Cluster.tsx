import { useTopology } from "../hooks/useTopology";
import { LabLayout } from "../components/LabLayout";
import { TopologyView } from "../components/TopologyView";
import { SlotMap } from "../components/SlotMap";

export function ClusterPage() {
  const { data, refresh } = useTopology("cluster");
  return (
    <LabLayout
      lab="cluster"
      pods={data?.pods ?? []}
      topology={<TopologyView lab="cluster" data={data} />}
      extra={
        <div className="mt-2 space-y-2">
          <SlotMap shards={data?.shards ?? []} />
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
