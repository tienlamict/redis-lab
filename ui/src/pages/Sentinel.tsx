import { useTopology } from "../hooks/useTopology";
import { LabLayout } from "../components/LabLayout";
import { TopologyView } from "../components/TopologyView";

export function SentinelPage() {
  const { data, refresh } = useTopology("sentinel");
  return (
    <LabLayout
      lab="sentinel"
      pods={data?.pods ?? []}
      masterPod={data?.currentMaster}
      topology={<TopologyView lab="sentinel" data={data} />}
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
