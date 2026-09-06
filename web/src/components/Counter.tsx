import type { Stats } from "../api";
import { copy } from "../copy";
import { money, shortSignature } from "../format";

const external = "font-medium text-accent underline-offset-4 hover:underline";

export function Counter({ stats }: { stats: Stats | null }) {
  if (!stats) {
    return (
      <section aria-busy="true" className="rounded-md border border-rule bg-slip p-6 text-sm text-ink-2 sm:p-8">
        {copy.loading}
      </section>
    );
  }
  const percent = stats.capCents > 0 ? Math.min(100, (stats.pledgedCents / stats.capCents) * 100) : 0;

  return (
    <section aria-labelledby="pledge-heading" className="rounded-md border border-rule bg-slip p-6 sm:p-8">
      <h2 id="pledge-heading" className="sr-only">
        The pledge so far
      </h2>
      <div aria-live="polite" aria-atomic="true" className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <p className="flex items-baseline gap-3">
          <span
            key={stats.count}
            className="inline-block font-mono text-6xl font-medium leading-none tabular-nums motion-safe:animate-tick sm:text-7xl"
          >
            {stats.count}
          </span>
          <span className="text-sm uppercase tracking-[0.14em] text-ink-2">{stats.count === 1 ? "link" : "links"}</span>
        </p>
        <p className="font-mono text-2xl leading-none tabular-nums">
          {money(stats.pledgedCents)}{" "}
          <span className="font-sans text-sm tracking-normal text-ink-2">pledged of {money(stats.capCents)}</span>
        </p>
      </div>

      <div
        role="progressbar"
        aria-label="Pledged so far"
        aria-valuemin={0}
        aria-valuemax={stats.capCents}
        aria-valuenow={stats.pledgedCents}
        aria-valuetext={`${money(stats.pledgedCents)} of ${money(stats.capCents)}`}
        className="mt-5 h-1.5 w-full overflow-hidden rounded-full bg-accent-soft"
      >
        <div className="h-full rounded-full bg-accent transition-[width] duration-700 ease-out" style={{ width: `${percent}%` }} />
      </div>

      <p className="mt-4 text-[15px]">
        →{" "}
        <a href={stats.charity.url} className={external} target="_blank" rel="noopener noreferrer">
          {stats.charity.name}
        </a>
      </p>

      {(stats.head || stats.signer) && (
        <dl className="mt-5 flex flex-wrap gap-x-8 gap-y-1 font-mono text-xs text-ink-2">
          {stats.head && (
            <div className="flex gap-2">
              <dt>Chain head</dt>
              <dd>
                <a href={stats.head.explorerUrl} className={external} target="_blank" rel="noopener noreferrer">
                  #{stats.head.n} · {shortSignature(stats.head.signature)}
                </a>
              </dd>
            </div>
          )}
          {stats.signer && (
            <div className="flex gap-2">
              <dt>Signer</dt>
              <dd>
                <a href={stats.signer.explorerUrl} className={external} target="_blank" rel="noopener noreferrer">
                  {shortSignature(stats.signer.address)}
                </a>
              </dd>
            </div>
          )}
        </dl>
      )}

      {stats.paused && (
        <p role="status" className="mt-4 rounded border border-rule bg-paper px-3 py-2 text-sm">
          {copy.paused}
        </p>
      )}

      <details className="mt-6 border-t border-rule pt-4 text-[15px] leading-relaxed">
        <summary className="cursor-pointer select-none font-medium">{copy.howItWorks}</summary>
        <ol className="mt-3 list-decimal space-y-2 pl-5 text-ink-2">
          <li>You add one sentence. No signup, no wallet.</li>
          <li>
            Each link is written to Solana {stats.cluster} as a memo that references the previous link — the chain is
            public and verifiable.
          </li>
          <li>
            For every confirmed link I donate {money(stats.perLinkCents)} to the {stats.charity.name}, up to{" "}
            {money(stats.capCents)}. The chain is the receipt.
          </li>
        </ol>
      </details>
    </section>
  );
}
