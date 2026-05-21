// Thin fetch wrappers. Paths are relative — Vite dev proxy + embedded prod
// both serve them off the same origin.
import type {
  CommandResponse,
  KillResponse,
  Lab,
  StandaloneTopology,
  SentinelTopology,
  ClusterTopology,
} from "./types";

async function getJson<T>(url: string): Promise<T> {
  const r = await fetch(url);
  if (!r.ok) throw new Error(`${url}: ${r.status}`);
  return r.json();
}

async function postJson<T>(url: string, body: unknown): Promise<T> {
  const r = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(`${url}: ${r.status}`);
  return r.json();
}

export const api = {
  topologyStandalone: () =>
    getJson<StandaloneTopology>("/api/labs/standalone/topology"),
  topologySentinel: () =>
    getJson<SentinelTopology>("/api/labs/sentinel/topology"),
  topologyCluster: () => getJson<ClusterTopology>("/api/labs/cluster/topology"),

  runCommand: (lab: Lab, command: string) =>
    postJson<CommandResponse>(`/api/labs/${lab}/command`, { command }),

  killPod: (lab: Lab, target: string) =>
    postJson<KillResponse>(`/api/labs/${lab}/fault/kill`, { target }),

  resetLab: (lab: Lab) =>
    postJson<KillResponse>(`/api/labs/${lab}/reset`, {}),
};
