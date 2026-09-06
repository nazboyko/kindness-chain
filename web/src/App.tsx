import { useCallback, useEffect, useRef, useState } from "react";
import { api, type Link, type Stats } from "./api";
import { Counter } from "./components/Counter";
import { Feed } from "./components/Feed";
import { Footer } from "./components/Footer";
import { LinkForm } from "./components/LinkForm";
import { copy } from "./copy";
import { useEvents, useTick } from "./events";

function linkFromQuery(): number | null {
  const raw = new URLSearchParams(window.location.search).get("link");
  if (raw === null) return null;
  const n = Number(raw);
  return Number.isInteger(n) && n >= 0 ? n : null;
}

function byNewest(a: Link, b: Link): number {
  return b.n - a.n;
}

export function App() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [links, setLinks] = useState<Link[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [mine, setMine] = useState<Link | null>(null);
  const [fresh, setFresh] = useState<Set<number>>(() => new Set());
  const [highlight] = useState<number | null>(linkFromQuery);
  const known = useRef(new Set<number>());
  const loadedOnce = useRef(false);
  const now = useTick(30_000);

  // links that arrive after the first paint are the ones worth a motion
  const notice = useCallback((link: Link) => {
    if (known.current.has(link.n)) return;
    known.current.add(link.n);
    if (loadedOnce.current) setFresh((prev) => new Set(prev).add(link.n));
  }, []);

  const upsert = useCallback(
    (link: Link) => {
      notice(link);
      setLinks((prev) => {
        const at = prev.findIndex((l) => l.n === link.n);
        if (at >= 0) {
          const next = prev.slice();
          next[at] = link;
          return next;
        }
        if (prev.length === 0 || link.n > prev[0].n) return [link, ...prev];
        return [...prev, link].sort(byNewest);
      });
    },
    [notice],
  );

  const loadFirstPage = useCallback(async () => {
    try {
      const [fetchedStats, page] = await Promise.all([api.stats(), api.links()]);
      page.links.forEach(notice);
      setStats(fetchedStats);
      setLinks(page.links);
      setHasMore(page.hasMore);
      setLoadFailed(false);
      loadedOnce.current = true;
    } catch {
      setLoadFailed(true);
    } finally {
      setLoading(false);
    }
  }, [notice]);

  useEffect(() => {
    void loadFirstPage();
  }, [loadFirstPage]);

  useEvents({
    onStats: setStats,
    onLink: (link) => {
      upsert(link);
      setMine((current) => (current && current.n === link.n ? link : current));
    },
    onReconnect: () => void loadFirstPage(),
  });

  // a link named in the address that is not on the first page is fetched
  // on its own, then the page scrolls to it
  const highlightShown = highlight !== null && links.some((l) => l.n === highlight);
  useEffect(() => {
    if (highlight === null || loading || highlightShown) return;
    api
      .link(highlight)
      .then(upsert)
      .catch(() => {});
  }, [highlight, loading, highlightShown, upsert]);

  useEffect(() => {
    if (!highlightShown) return;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    document.getElementById(`link-${highlight}`)?.scrollIntoView({ block: "center", behavior: reduced ? "auto" : "smooth" });
  }, [highlight, highlightShown]);

  async function loadOlder() {
    const oldest = links[links.length - 1];
    if (!oldest) return;
    setLoading(true);
    try {
      const page = await api.links(oldest.n);
      page.links.forEach((l) => known.current.add(l.n));
      setLinks((prev) => [...prev, ...page.links.filter((l) => !prev.some((p) => p.n === l.n))].sort(byNewest));
      setHasMore(page.hasMore);
    } catch {
      setLoadFailed(true);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="mx-auto max-w-[42rem] px-5 py-10 sm:px-8 sm:py-14">
      <header className="flex items-baseline justify-between">
        <h1 className="text-[13px] font-medium tracking-[0.16em] uppercase">{copy.title}</h1>
        {stats && <span className="font-mono text-xs text-ink-2">Solana {stats.cluster}</span>}
      </header>

      <p className="mt-10 font-serif text-[2rem] leading-[1.12] sm:mt-12 sm:text-[2.6rem]">
        {copy.tagline.map((line, i) => (
          <span key={line} className={i === 2 ? "block text-accent" : "block"}>
            {line}
          </span>
        ))}
      </p>

      <div className="mt-10">
        <Counter stats={stats} />
      </div>

      <LinkForm
        mine={mine}
        onSubmitted={(link) => {
          setMine(link);
          upsert(link);
        }}
      />

      <Feed
        links={links}
        hasMore={hasMore}
        loading={loading}
        loadFailed={loadFailed}
        now={now}
        mineN={mine?.n ?? null}
        highlight={highlight}
        fresh={fresh}
        onLoadOlder={loadOlder}
      />

      <Footer />
    </main>
  );
}
