// Stateful history-aware command sender for RedisConsole. Caps history at 200.
import { useCallback, useState } from "react";
import { api } from "../api";
import type { CommandResponse, Lab } from "../types";

export interface HistoryEntry {
  command: string;
  response: CommandResponse;
  at: string;
}

export function useRedisCommand(lab: Lab) {
  const [history, setHistory] = useState<HistoryEntry[]>([]);

  const run = useCallback(
    async (command: string) => {
      let response: CommandResponse;
      try {
        response = await api.runCommand(lab, command);
      } catch (e) {
        response = { error: String(e) };
      }
      setHistory((h) =>
        [...h, { command, response, at: new Date().toLocaleTimeString() }].slice(-200),
      );
      return response;
    },
    [lab],
  );

  const clear = useCallback(() => setHistory([]), []);
  return { history, run, clear };
}
