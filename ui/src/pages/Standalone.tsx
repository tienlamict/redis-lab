import { useTopology } from "../hooks/useTopology";
import { LabLayout } from "../components/LabLayout";
import { TopologyView } from "../components/TopologyView";

export function StandalonePage() {
  const { data, refresh } = useTopology("standalone");
  const pods = data?.pods ?? [];
  const masterPod = data?.master?.pod || pods.find((p) => p.name.includes("master"))?.name;
  return (
    <LabLayout
      lab="standalone"
      pods={pods}
      masterPod={masterPod}
      topology={<TopologyView lab="standalone" data={data} />}
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
