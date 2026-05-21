// useTopology: fetches the lab topology on mount + every `refreshMs` + after
// every SSE event that suggests state changed. Returns the latest snapshot.
import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import type {
  ClusterTopology,
  Lab,
  SentinelTopology,
  StandaloneTopology,
} from "../types";

type TopoMap = {
  standalone: StandaloneTopology;
  sentinel: SentinelTopology;
  cluster: ClusterTopology;
};

export function useTopology<L extends Lab>(
  lab: L,
  refreshMs = 4000,
): {
  data: TopoMap[L] | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
} {
  const [data, setData] = useState<TopoMap[L] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      let r: unknown;
      if (lab === "standalone") r = await api.topologyStandalone();
      else if (lab === "sentinel") r = await api.topologySentinel();
      else r = await api.topologyCluster();
      setData(r as TopoMap[L]);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [lab]);

  useEffect(() => {
    setLoading(true);
    refresh();
    const id = setInterval(refresh, refreshMs);
    return () => clearInterval(id);
  }, [refresh, refreshMs]);

  return { data, loading, error, refresh };
}
