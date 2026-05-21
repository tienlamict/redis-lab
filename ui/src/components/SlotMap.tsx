// SlotMap: a horizontal ribbon showing how 16384 cluster slots are divided
// between shards. Each shard gets a distinct color; hovering a segment shows
// the exact range.
import type { ShardView } from "../types";

const PALETTE = [
  "#10b981",
  "#3b82f6",
  "#f97316",
  "#a855f7",
  "#ec4899",
  "#facc15",
];

const TOTAL_SLOTS = 16384;

export function SlotMap({ shards }: { shards: ShardView[] }) {
  const segments: { from: number; to: number; color: string; label: string }[] = [];
  shards.forEach((sh, idx) => {
    const color = PALETTE[idx % PALETTE.length];
    for (const r of sh.slotRanges) {
      segments.push({
        from: r[0],
        to: r[1],
        color,
        label: `${sh.masterPod || "(?)"}: ${r[0]}–${r[1]}`,
      });
    }
  });
  segments.sort((a, b) => a.from - b.from);

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-950/40 p-3">
      <div className="mb-2 flex items-center justify-between text-xs text-slate-400">
        <span>Slot map (16384 slots)</span>
        <div className="flex gap-2">
          {shards.map((sh, idx) => (
            <span key={sh.masterId} className="flex items-center gap-1">
              <span
                className="inline-block h-2 w-3 rounded"
                style={{ background: PALETTE[idx % PALETTE.length] }}
              />
              <span className="font-mono">{sh.masterPod}</span>
            </span>
          ))}
        </div>
      </div>
      <div className="flex h-6 w-full overflow-hidden rounded">
        {segments.map((s, i) => {
          const width = ((s.to - s.from + 1) / TOTAL_SLOTS) * 100;
          return (
            <div
              key={i}
              title={s.label}
              style={{ width: `${width}%`, background: s.color }}
              className="border-r border-slate-900/60 transition hover:brightness-125"
            />
          );
        })}
      </div>
    </div>
  );
}
