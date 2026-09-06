import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, ApiError, type Link } from "../api";
import { copy, limits } from "../copy";
import { characters } from "../format";
import { seal } from "../pow";

interface Props {
  mine: Link | null;
  onSubmitted(link: Link): void;
}

interface Message {
  tone: "info" | "success" | "error";
  text: string;
  href?: string;
  hrefLabel?: string;
}

const cooldownMs = 10_000;

export function LinkForm({ mine, onSubmitted }: Props) {
  const [act, setAct] = useState("");
  const [by, setBy] = useState("");
  const [website, setWebsite] = useState("");
  const [busy, setBusy] = useState(false);
  const [coolingDown, setCoolingDown] = useState(false);
  const [message, setMessage] = useState<Message | null>(null);
  const textarea = useRef<HTMLTextAreaElement>(null);
  const cooldown = useRef<number>(0);

  const count = characters(act.trim());
  const over = count > limits.maxAct;

  // the visitor's own link reports back through the event stream
  useEffect(() => {
    if (!mine) return;
    if (mine.status === "confirmed") {
      setMessage({ tone: "success", text: copy.confirmed(mine.n), href: mine.explorerUrl, hrefLabel: copy.verifyOnExplorer });
    } else if (mine.status === "failed") {
      setMessage({ tone: "error", text: copy.failed(mine.n) });
    }
  }, [mine]);

  useEffect(() => () => window.clearTimeout(cooldown.current), []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy || coolingDown) return;
    if (count < limits.minAct) {
      setMessage({ tone: "error", text: copy.tooShort });
      textarea.current?.focus();
      return;
    }
    if (over) {
      setMessage({ tone: "error", text: copy.tooLong });
      textarea.current?.focus();
      return;
    }
    setBusy(true);
    try {
      const link = await sealAndSend();
      onSubmitted(link);
      setMessage({ tone: "info", text: copy.added(link.n) });
      setAct("");
      setCoolingDown(true);
      cooldown.current = window.setTimeout(() => setCoolingDown(false), cooldownMs);
    } catch (err) {
      setMessage({ tone: "error", text: err instanceof ApiError ? err.message : copy.offline });
    } finally {
      setBusy(false);
    }
  }

  // the seal is computed in the browser; if the server has forgotten the
  // seed by the time it arrives, one fresh attempt is made quietly
  async function sealAndSend() {
    setMessage({ tone: "info", text: copy.sealing });
    try {
      return await api.add(act, by, website, await seal(act));
    } catch (err) {
      if (!(err instanceof ApiError && err.reason === "challenge")) throw err;
      return await api.add(act, by, website, await seal(act));
    }
  }

  const tone = message?.tone === "error" ? "text-failed" : message?.tone === "success" ? "text-accent" : "text-ink-2";

  return (
    <form onSubmit={submit} noValidate className="relative mt-12">
      <label htmlFor="act" className="block font-serif text-[1.35rem] leading-snug">
        {copy.fieldLabel}
      </label>
      <textarea
        id="act"
        name="act"
        ref={textarea}
        value={act}
        onChange={(e) => setAct(e.target.value)}
        rows={3}
        required
        placeholder={copy.namePlaceholder}
        aria-describedby="act-count act-message"
        aria-invalid={message?.tone === "error" || undefined}
        className="mt-3 block w-full resize-y rounded-md border border-rule bg-slip px-4 py-3 font-serif text-lg leading-snug placeholder:text-ink-2/60 focus:border-accent"
      />
      <p id="act-count" className={`mt-1 text-right font-mono text-xs tabular-nums ${over ? "text-failed" : "text-ink-2"}`}>
        {count} / {limits.maxAct}
      </p>

      <label htmlFor="by" className="mt-4 block text-sm font-medium text-ink-2">
        {copy.nameLabel}
      </label>
      <input
        id="by"
        name="by"
        value={by}
        onChange={(e) => setBy(e.target.value)}
        maxLength={limits.maxName}
        autoComplete="nickname"
        className="mt-2 block w-full max-w-xs rounded-md border border-rule bg-slip px-3 py-2.5 text-[15px] focus:border-accent"
      />

      {/* people never see this field; a script that fills every input does */}
      <div aria-hidden="true" className="absolute -left-[9999px] top-0 h-px w-px overflow-hidden">
        <label htmlFor="website">Website</label>
        <input
          id="website"
          name="website"
          tabIndex={-1}
          autoComplete="off"
          value={website}
          onChange={(e) => setWebsite(e.target.value)}
        />
      </div>

      <button
        type="submit"
        disabled={busy || coolingDown}
        className="mt-6 min-h-[44px] rounded-md bg-accent px-6 py-2.5 font-medium text-slip transition-colors hover:bg-accent-deep disabled:cursor-default disabled:opacity-60"
      >
        {copy.button}
      </button>
      <p id="act-message" role="status" aria-live="polite" className={`mt-3 min-h-6 text-[15px] ${tone}`}>
        {message?.text}
        {message?.href && (
          <>
            {" "}
            <a href={message.href} target="_blank" rel="noopener noreferrer" className="font-medium underline underline-offset-4">
              {message.hrefLabel}
            </a>
          </>
        )}
      </p>
    </form>
  );
}
