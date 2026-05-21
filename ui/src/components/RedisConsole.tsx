// Terminal-style command input with history (Arrow Up/Down), CLEAR shortcut,
// and per-result metadata display (slot/executedOn/redirects in cluster mode).
import { useRef, useState, KeyboardEvent } from "react";
import type { Lab } from "../types";
import { useRedisCommand, type HistoryEntry } from "../hooks/useRedisCommand";

function formatResult(r: HistoryEntry["response"]): string {
  if (r.error) return `(error) ${r.error}`;
  const v = r.result;
  if (v === null || v === undefined) return "(nil)";
  if (typeof v === "string") return v;
  if (Array.isArray(v)) return v.map((x) => String(x)).join("\n");
  return JSON.stringify(v);
}

export function RedisConsole({ lab }: { lab: Lab }) {
  const { history, run, clear } = useRedisCommand(lab);
  const [input, setInput] = useState("");
  const [cursor, setCursor] = useState<number | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const onKey = async (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      const cmd = input.trim();
      if (!cmd) return;
      if (cmd.toUpperCase() === "CLEAR") {
        clear();
        setInput("");
        return;
      }
      setInput("");
      setCursor(null);
      await run(cmd);
      requestAnimationFrame(() => {
        scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
      });
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (!history.length) return;
      const next = cursor === null ? history.length - 1 : Math.max(0, cursor - 1);
      setCursor(next);
      setInput(history[next].command);
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      if (cursor === null) return;
      const next = cursor + 1;
      if (next >= history.length) {
        setCursor(null);
        setInput("");
      } else {
        setCursor(next);
        setInput(history[next].command);
      }
    }
  };

  return (
    <div className="flex h-full flex-col rounded-lg border border-slate-800 bg-black/40">
      <div className="flex items-center justify-between border-b border-slate-800 px-3 py-1.5 text-xs text-slate-400">
        <span>redis-cli :: {lab}</span>
        <span className="opacity-60">type CLEAR to clear · ↑↓ history</span>
      </div>
      <div ref={scrollRef} className="flex-1 overflow-y-auto px-3 py-2 font-mono text-sm">
        {history.length === 0 && (
          <div className="text-slate-500">
            Try: <span className="text-slate-300">SET foo bar</span>,{" "}
            <span className="text-slate-300">GET foo</span>
            {lab === "cluster" && (
              <>
                , <span className="text-slate-300">SET user:{"{42}"}:profile alice</span>
              </>
            )}
          </div>
        )}
        {history.map((h, i) => (
          <div key={i} className="mb-2">
            <div className="text-emerald-300">
              <span className="text-slate-500">[{h.at}]</span>{" "}
              <span className="text-emerald-400">&gt;</span> {h.command}
            </div>
            <pre className="whitespace-pre-wrap text-slate-100">{formatResult(h.response)}</pre>
            {(h.response.slot !== undefined || h.response.executedOn) && (
              <div className="text-xs text-slate-500">
                {h.response.slot !== undefined && <>slot={h.response.slot} </>}
                {h.response.executedOn && <>· executed on {h.response.executedOn}</>}
              </div>
            )}
            {h.response.redirects && h.response.redirects.length > 0 && (
              <div className="text-xs text-amber-300/80">
                {h.response.redirects.map((r, j) => (
                  <div key={j}>
                    {r.kind} {r.slot} → {r.to}
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
      <div className="flex items-center border-t border-slate-800 px-3 py-2">
        <span className="mr-2 text-emerald-400">&gt;</span>
        <input
          className="flex-1 bg-transparent font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:outline-none"
          placeholder="redis command"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={onKey}
          autoFocus
        />
      </div>
    </div>
  );
}
