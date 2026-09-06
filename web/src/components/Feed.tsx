import { useState } from "react";
import { api, ApiError, type Link, type Status, type Verification } from "../api";
import { copy, linkAddress, shareOpening } from "../copy";
import { absoluteTime, relativeTime, shortSignature } from "../format";

interface Props {
  links: Link[];
  hasMore: boolean;
  loading: boolean;
  loadFailed: boolean;
  now: number;
  mineN: number | null;
  highlight: number | null;
  fresh: Set<number>;
  onLoadOlder(): void;
}

export function Feed({ links, hasMore, loading, loadFailed, now, mineN, highlight, fresh, onLoadOlder }: Props) {
  return (
    <section aria-labelledby="feed-heading" className="mt-16">
      <div className="flex items-baseline justify-between">
        <h2 id="feed-heading" className="text-sm font-medium uppercase tracking-[0.14em] text-ink-2">
          {copy.feedHeading}
        </h2>
        <span className="text-xs text-ink-2">{copy.feedOrder}</span>
      </div>
      {loadFailed && <p className="mt-6 text-failed">{copy.loadFailed}</p>}
      {!loadFailed && loading && links.length === 0 && <p className="mt-6 text-ink-2">{copy.loading}</p>}
      <ol className="relative mt-4 before:absolute before:top-4 before:bottom-4 before:left-[7px] before:w-px before:bg-rule before:content-['']">
        {links.map((link) => (
          <LinkRow
            key={link.n}
            link={link}
            now={now}
            mine={link.n === mineN}
            highlighted={link.n === highlight}
            fresh={fresh.has(link.n)}
          />
        ))}
      </ol>
      {hasMore && (
        <button
          type="button"
          onClick={onLoadOlder}
          disabled={loading}
          className="mt-6 ml-8 min-h-[44px] rounded-md border border-rule bg-slip px-5 font-medium transition-colors hover:border-accent disabled:opacity-60"
        >
          {copy.loadOlder}
        </button>
      )}
    </section>
  );
}

interface RowProps {
  link: Link;
  now: number;
  mine: boolean;
  highlighted: boolean;
  fresh: boolean;
}

type Check = "checking" | Verification | { error: string } | null;

const action = "-mx-1 inline-flex min-h-8 items-center rounded px-1 font-medium text-accent underline-offset-4 hover:underline";

function LinkRow({ link, now, mine, highlighted, fresh }: RowProps) {
  const [check, setCheck] = useState<Check>(null);
  const [copied, setCopied] = useState(false);

  async function verify() {
    setCheck("checking");
    try {
      setCheck(await api.verify(link.n));
    } catch (err) {
      setCheck({ error: err instanceof ApiError ? err.message : copy.offline });
    }
  }

  async function share() {
    const text = shareOpening(link.n, mine);
    const url = linkAddress(link.n);
    if (navigator.share) {
      try {
        await navigator.share({ text, url });
      } catch {
        // the visitor closed the sheet
      }
      return;
    }
    try {
      await navigator.clipboard.writeText(`${text} ${url}`);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // the clipboard is blocked; nothing else to do
    }
  }

  const tint = highlighted ? "motion-reduce:bg-accent-soft motion-safe:animate-fade-tint" : "";
  const rise = fresh ? "motion-safe:animate-rise" : "";

  return (
    <li id={`link-${link.n}`} className={`relative border-b border-rule py-5 pl-8 ${tint} ${rise}`}>
      <Marker status={link.status} />
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="font-mono text-sm text-ink-2">#{link.n}</span>
        {link.n === 0 && <span className="text-xs uppercase tracking-[0.12em] text-accent">the pledge</span>}
        <StatusChip status={link.status} />
      </div>
      <p className="mt-1.5 font-serif text-[19px] leading-snug">{link.act}</p>
      <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-2">
        <span>{link.by || "Anonymous"}</span>
        <time dateTime={link.createdAt} title={absoluteTime(link.createdAt)}>
          {relativeTime(link.createdAt, now)}
        </time>
        {link.status === "confirmed" && (
          <>
            <button type="button" onClick={verify} className={action}>
              {copy.verify}
            </button>
            <a href={link.explorerUrl} className={action} target="_blank" rel="noopener noreferrer">
              {copy.explorer}
            </a>
          </>
        )}
        <button type="button" onClick={share} className={action}>
          {copied ? copy.copied : copy.share}
        </button>
      </div>
      <CheckResult check={check} />
    </li>
  );
}

function Marker({ status }: { status: Status }) {
  const base = "absolute left-0 top-[1.55rem] h-4 w-4 rounded-full border-2";
  if (status === "confirmed") return <span aria-hidden="true" className={`${base} border-accent bg-accent`} />;
  if (status === "failed") return <span aria-hidden="true" className={`${base} border-failed bg-paper`} />;
  return <span aria-hidden="true" className={`${base} border-accent bg-paper motion-safe:animate-breathe`} />;
}

function StatusChip({ status }: { status: Status }) {
  switch (status) {
    case "confirmed":
      return <span className="rounded-full bg-accent-soft px-2 py-0.5 text-xs font-medium text-accent">{copy.confirmedChip}</span>;
    case "failed":
      return <span className="rounded-full bg-failed-soft px-2 py-0.5 text-xs font-medium text-failed">{copy.failedChip}</span>;
    default:
      return <span className="rounded-full border border-rule px-2 py-0.5 text-xs text-ink-2">{copy.pendingChip}</span>;
  }
}

function CheckResult({ check }: { check: Check }) {
  if (check === null) return null;
  if (check === "checking") return <p className="mt-2 text-sm text-ink-2">{copy.checking}</p>;
  if ("error" in check) return <p className="mt-2 text-sm text-failed">{check.error}</p>;
  return (
    <div className="mt-3 rounded-md border border-rule bg-slip p-3 text-sm">
      {check.matches ? (
        <p className="font-medium text-accent">{copy.matches}</p>
      ) : (
        <p className="font-medium text-failed">{check.problem ?? copy.differs}</p>
      )}
      <p className="mt-1 text-ink-2">
        memo #{check.n} · prev {shortSignature(check.stored.prev)} ·{" "}
        <a href={check.explorerUrl} className={action} target="_blank" rel="noopener noreferrer">
          {copy.explorer}
        </a>
      </p>
      {check.raw && (
        <details className="mt-2">
          <summary className="cursor-pointer text-ink-2">{copy.showMemo}</summary>
          <pre className="mt-2 overflow-x-auto font-mono text-xs break-all whitespace-pre-wrap">{check.raw}</pre>
        </details>
      )}
    </div>
  );
}
