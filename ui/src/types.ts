// Types mirror backend JSON shapes (see backend/internal/api/topology.go).
// Keep in sync — backend struct tags determine JSON keys.

export type Lab = "standalone" | "sentinel" | "cluster";

export interface PodSummary {
  name: string;
  ip: string;
  status: string;
  ready: boolean;
  node: string;
}

export interface NodeView {
  pod: string;
  ip: string;
  role?: string;
  status: string; // "ok" | "pending" | "sdown" | "odown" | "disconnected" | "unknown" | "error"
  replOffset?: number;
}

export interface SentinelView {
  pod: string;
  status: string;
  monitoring: string;
}

export interface NodeRef {
  id: string;
  pod: string;
}

export interface ShardView {
  masterId: string;
  masterPod: string;
  slotRanges: [number, number][];
  replicas: NodeRef[];
}

export interface RawNodeInfo {
  id: string;
  addr: string;
  flags: string;
  masterId?: string;
  slotRanges?: [number, number][];
}

export interface StandaloneTopology {
  master: NodeView;
  replicas: NodeView[];
  pods: PodSummary[];
  info?: Record<string, string>;
  error?: string;
}

export interface SentinelTopology {
  master: NodeView;
  replicas: NodeView[];
  sentinels: SentinelView[];
  currentMaster: string;
  pods: PodSummary[];
  masterError?: string;
  replicasError?: string;
  sentinelsError?: string;
}

export interface ClusterTopology {
  shards: ShardView[];
  nodes: RawNodeInfo[];
  pods: PodSummary[];
  info?: Record<string, string>;
  error?: string;
}

export interface Redirect {
  kind: "MOVED" | "ASK";
  slot: number;
  to: string;
}

export interface CommandResponse {
  result?: unknown;
  error?: string;
  executedOn?: string;
  slot?: number;
  redirects?: Redirect[];
}

export interface KillResponse {
  ok: boolean;
  message: string;
  error?: string;
}

// SSE event payload as emitted by backend events.Event.
export interface SseEvent {
  lab: Lab;
  type:
    | "hello"
    | "pod_added"
    | "pod_modified"
    | "pod_deleted"
    | "role_changed"
    | "sentinel_event"
    | "cluster_event"
    | "pod_event";
  time: string;
  payload?: Record<string, unknown>;
}
