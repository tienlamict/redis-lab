// TopologyView: per-lab SVG rendering of nodes + replication/monitor arrows.
// Pure SVG (no react-flow). Click a node → onNodeClick(podName).
// Visual rules per spec:
//   green   = master healthy
//   blue    = replica healthy
//   violet  = sentinel
//   amber   = sdown / pending
//   red     = odown / deleted / disconnected / error
// Arrow style: solid "repl" for replication, dashed "monitor" for sentinel→redis.
import { useEffect, useRef, useState } from "react";
import type {
  ClusterTopology,
  Lab,
  NodeView,
  SentinelTopology,
  SentinelView,
  ShardView,
  StandaloneTopology,
} from "../types";

const COLOR = {
  masterFill: "#065f46",
  masterStroke: "#34d399",
  replicaFill: "#0c4a6e",
  replicaStroke: "#38bdf8",
  sentinelFill: "#4c1d95",
  sentinelStroke: "#c4b5fd",
  pendingFill: "#78350f",
  pendingStroke: "#fbbf24",
  errorFill: "#7f1d1d",
  errorStroke: "#f87171",
  arrow: "#94a3b8",
  text: "#e2e8f0",
  selected: "#fbbf24",
};

function nodeColors(status: string, role?: string) {
  if (status === "sdown" || status === "pending")
    return { fill: COLOR.pendingFill, stroke: COLOR.pendingStroke };
  if (status === "odown" || status === "disconnected" || status === "error")
    return { fill: COLOR.errorFill, stroke: COLOR.errorStroke };
  if (role === "master") return { fill: COLOR.masterFill, stroke: COLOR.masterStroke };
  if (role === "sentinel") return { fill: COLOR.sentinelFill, stroke: COLOR.sentinelStroke };
  return { fill: COLOR.replicaFill, stroke: COLOR.replicaStroke };
}

// Spec calls for 140 × 60.
const NODE_W = 140;
const NODE_H = 60;

function SvgNode(props: {
  x: number;
  y: number;
  title: string;
  subtitle?: string;
  role?: string;
  status: string;
  selected?: boolean;
  flash?: boolean;
  onClick?: () => void;
}) {
  const c = nodeColors(props.status, props.role);
  return (
    <g
      transform={`translate(${props.x}, ${props.y})`}
      className={`fade-in ${props.flash ? "flash-bg" : ""} cursor-pointer`}
      onClick={props.onClick}
    >
      <rect
        rx={10}
        ry={10}
        width={NODE_W}
        height={NODE_H}
        fill={c.fill}
        stroke={props.selected ? COLOR.selected : c.stroke}
        strokeWidth={props.selected ? 3 : 1.5}
      />
      <text
        x={NODE_W / 2}
        y={24}
        textAnchor="middle"
        fontSize={11}
        fontFamily="ui-monospace, monospace"
        fontWeight={600}
        fill={COLOR.text}
      >
        {props.title}
      </text>
      {props.subtitle && (
        <text
          x={NODE_W / 2}
          y={42}
          textAnchor="middle"
          fontSize={10}
          fontFamily="ui-monospace, monospace"
          fill="#cbd5e1"
        >
          {props.subtitle}
        </text>
      )}
    </g>
  );
}

function Arrow(props: {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  label?: string;
  dashed?: boolean;
}) {
  const mx = (props.x1 + props.x2) / 2;
  const my = (props.y1 + props.y2) / 2;
  return (
    <g>
      <line
        x1={props.x1}
        y1={props.y1}
        x2={props.x2}
        y2={props.y2}
        stroke={COLOR.arrow}
        strokeWidth={1.5}
        markerEnd="url(#arrowhead)"
        strokeDasharray={props.dashed ? "6 4" : undefined}
      />
      {props.label && (
        <text x={mx + 4} y={my - 4} fontSize={10} fill={COLOR.arrow}>
          {props.label}
        </text>
      )}
    </g>
  );
}

function ArrowDefs() {
  return (
    <defs>
      <marker
        id="arrowhead"
        viewBox="0 0 10 10"
        refX="8"
        refY="5"
        markerWidth="6"
        markerHeight="6"
        orient="auto-start-reverse"
      >
        <path d="M 0 0 L 10 5 L 0 10 z" fill={COLOR.arrow} />
      </marker>
    </defs>
  );
}

// useFlash tracks which pods changed role recently and returns a Set the
// renderer queries to apply the flash-bg animation. The Set is rebuilt every
// time `signature` changes; entries expire after 800ms.
function useFlash(signature: string) {
  const prevRef = useRef<string | null>(null);
  const [flashing, setFlashing] = useState<Set<string>>(new Set());
  useEffect(() => {
    if (prevRef.current === null) {
      prevRef.current = signature;
      return;
    }
    if (prevRef.current === signature) return;
    // Detect changed pod->role pairs.
    const prev = new Map(prevRef.current.split(",").map((s) => s.split("=") as [string, string]));
    const curr = new Map(signature.split(",").map((s) => s.split("=") as [string, string]));
    const changed = new Set<string>();
    curr.forEach((role, pod) => {
      if (prev.get(pod) && prev.get(pod) !== role) changed.add(pod);
    });
    prevRef.current = signature;
    if (changed.size > 0) {
      setFlashing(changed);
      const t = setTimeout(() => setFlashing(new Set()), 800);
      return () => clearTimeout(t);
    }
  }, [signature]);
  return flashing;
}

// ---------- per-lab layouts ----------

function StandaloneLayout({
  d,
  onNodeClick,
  selected,
}: {
  d: StandaloneTopology;
  onNodeClick?: (pod: string) => void;
  selected?: string | null;
}) {
  const W = 720;
  const H = 280;
  const masterX = W / 2 - NODE_W / 2;
  const masterY = 16;
  const replicaY = 180;
  const reps = d.replicas ?? [];
  const gap = reps.length > 1 ? (W - NODE_W * reps.length) / (reps.length + 1) : (W - NODE_W) / 2;

  const sig =
    `${d.master?.pod}=${d.master?.role}` +
    "," +
    reps.map((r) => `${r.pod}=${r.role}`).join(",");
  const flashing = useFlash(sig);

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full max-h-[360px]">
      <ArrowDefs />
      <SvgNode
        x={masterX}
        y={masterY}
        title={d.master?.pod || "(no master)"}
        subtitle={`master · ${d.master?.status || "?"}`}
        role="master"
        status={d.master?.status || "unknown"}
        selected={selected === d.master?.pod}
        flash={flashing.has(d.master?.pod ?? "")}
        onClick={() => d.master?.pod && onNodeClick?.(d.master.pod)}
      />
      {reps.map((r, i) => {
        const x = gap * (i + 1) + NODE_W * i;
        return (
          <g key={r.pod || i}>
            <SvgNode
              x={x}
              y={replicaY}
              title={r.pod}
              subtitle={`replica · ${r.status}`}
              role="slave"
              status={r.status}
              selected={selected === r.pod}
              flash={flashing.has(r.pod)}
              onClick={() => onNodeClick?.(r.pod)}
            />
            <Arrow
              x1={masterX + NODE_W / 2}
              y1={masterY + NODE_H}
              x2={x + NODE_W / 2}
              y2={replicaY}
              label="repl"
            />
          </g>
        );
      })}
    </svg>
  );
}

function SentinelLayout({
  d,
  onNodeClick,
  selected,
}: {
  d: SentinelTopology;
  onNodeClick?: (pod: string) => void;
  selected?: string | null;
}) {
  const W = 760;
  const H = 320;
  const podByName = new Map<string, NodeView>();
  if (d.master?.pod) podByName.set(d.master.pod, d.master);
  for (const r of d.replicas || []) podByName.set(r.pod, r);
  const redisPods = (d.pods || []).map((p) => p.name);
  const sentinels: SentinelView[] = d.sentinels || [];
  const sentinelPods = new Set(sentinels.map((s) => s.pod));

  const redisY = 16;
  const sentinelY = 180;
  const gapR = redisPods.length > 1 ? (W - NODE_W * redisPods.length) / (redisPods.length + 1) : (W - NODE_W) / 2;

  const sig = redisPods.map((n) => `${n}=${d.currentMaster === n ? "master" : "slave"}`).join(",");
  const flashing = useFlash(sig);

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full max-h-[400px]">
      <ArrowDefs />
      {redisPods.map((name, i) => {
        const x = gapR * (i + 1) + NODE_W * i;
        const isMaster = d.currentMaster === name;
        const meta = podByName.get(name);
        return (
          <g key={name}>
            <SvgNode
              x={x}
              y={redisY}
              title={name}
              subtitle={`${isMaster ? "master" : "replica"} · ${meta?.status || "?"}`}
              role={isMaster ? "master" : "slave"}
              status={meta?.status || "unknown"}
              selected={selected === name}
              flash={flashing.has(name)}
              onClick={() => onNodeClick?.(name)}
            />
            {sentinelPods.has(name) && (
              <Arrow
                x1={x + NODE_W / 2}
                y1={sentinelY}
                x2={x + NODE_W / 2}
                y2={redisY + NODE_H + 2}
                dashed
                label="monitor"
              />
            )}
          </g>
        );
      })}
      {sentinels.map((s, i) => {
        const idx = redisPods.indexOf(s.pod);
        const baseX = idx >= 0 ? gapR * (idx + 1) + NODE_W * idx : 16 + i * (NODE_W + 16);
        return (
          <SvgNode
            key={s.pod || i}
            x={baseX}
            y={sentinelY}
            title={s.pod}
            subtitle={`sentinel · ${s.status}`}
            role="sentinel"
            status={s.status}
            selected={selected === s.pod}
            onClick={() => onNodeClick?.(s.pod)}
          />
        );
      })}
    </svg>
  );
}

function ClusterLayout({
  d,
  onNodeClick,
  selected,
}: {
  d: ClusterTopology;
  onNodeClick?: (pod: string) => void;
  selected?: string | null;
}) {
  const shards: ShardView[] = d.shards || [];
  const W = 820;
  const H = 320;
  const masterY = 16;
  const replicaY = 180;
  const gap = shards.length > 0 ? (W - NODE_W * shards.length) / (shards.length + 1) : 0;
  const sig = shards
    .flatMap((sh) => [
      `${sh.masterPod}=master`,
      ...sh.replicas.map((r) => `${r.pod}=replica`),
    ])
    .join(",");
  const flashing = useFlash(sig);
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full max-h-[400px]">
      <ArrowDefs />
      {shards.map((sh, i) => {
        const x = gap * (i + 1) + NODE_W * i;
        const slotLabel = sh.slotRanges.map((r) => `${r[0]}-${r[1]}`).join(", ");
        const masterStatus =
          (d.pods || []).find((p) => p.name === sh.masterPod)?.ready ? "ok" : "pending";
        return (
          <g key={sh.masterId}>
            <SvgNode
              x={x}
              y={masterY}
              title={sh.masterPod || "(?)"}
              subtitle={`slots ${slotLabel}`}
              role="master"
              status={masterStatus}
              selected={selected === sh.masterPod}
              flash={flashing.has(sh.masterPod)}
              onClick={() => sh.masterPod && onNodeClick?.(sh.masterPod)}
            />
            {sh.replicas.map((r) => {
              const ready = (d.pods || []).find((p) => p.name === r.pod)?.ready;
              return (
                <g key={r.id}>
                  <SvgNode
                    x={x}
                    y={replicaY}
                    title={r.pod || "(?)"}
                    subtitle="replica"
                    role="slave"
                    status={ready ? "ok" : "pending"}
                    selected={selected === r.pod}
                    flash={flashing.has(r.pod)}
                    onClick={() => r.pod && onNodeClick?.(r.pod)}
                  />
                  <Arrow
                    x1={x + NODE_W / 2}
                    y1={masterY + NODE_H}
                    x2={x + NODE_W / 2}
                    y2={replicaY}
                    label="repl"
                  />
                </g>
              );
            })}
          </g>
        );
      })}
    </svg>
  );
}

export function TopologyView(props: {
  lab: Lab;
  data: StandaloneTopology | SentinelTopology | ClusterTopology | null;
  onNodeClick?: (pod: string) => void;
  selected?: string | null;
}) {
  if (!props.data) {
    return (
      <div className="flex h-48 items-center justify-center text-slate-500">
        Loading topology…
      </div>
    );
  }
  if (props.lab === "standalone")
    return (
      <StandaloneLayout
        d={props.data as StandaloneTopology}
        onNodeClick={props.onNodeClick}
        selected={props.selected}
      />
    );
  if (props.lab === "sentinel")
    return (
      <SentinelLayout
        d={props.data as SentinelTopology}
        onNodeClick={props.onNodeClick}
        selected={props.selected}
      />
    );
  return (
    <ClusterLayout
      d={props.data as ClusterTopology}
      onNodeClick={props.onNodeClick}
      selected={props.selected}
    />
  );
}
