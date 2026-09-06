import { useEffect, useRef, useState } from "react";
import type { Link, Stats } from "./api";

interface Handlers {
  onStats(stats: Stats): void;
  onLink(link: Link): void;
  onReconnect(): void;
}

// useEvents keeps one event stream open for the life of the page. The
// browser reconnects on its own; after a gap the page is told, so it
// can refetch and never show stale numbers.
export function useEvents(handlers: Handlers) {
  const latest = useRef(handlers);
  useEffect(() => {
    latest.current = handlers;
  });
  useEffect(() => {
    const source = new EventSource("/api/events");
    let dropped = false;
    source.addEventListener("stats", (e) => latest.current.onStats(JSON.parse((e as MessageEvent).data)));
    source.addEventListener("link", (e) => latest.current.onLink(JSON.parse((e as MessageEvent).data)));
    source.onerror = () => {
      dropped = true;
    };
    source.onopen = () => {
      if (dropped) {
        dropped = false;
        latest.current.onReconnect();
      }
    };
    return () => source.close();
  }, []);
}

// useTick re-renders on an interval, so relative times keep aging.
export function useTick(ms: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), ms);
    return () => window.clearInterval(id);
  }, [ms]);
  return now;
}
