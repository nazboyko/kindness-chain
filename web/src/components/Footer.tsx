import { copy } from "../copy";

const external = "text-accent underline-offset-4 hover:underline";

export function Footer() {
  return (
    <footer className="mt-16 border-t border-rule pt-6 text-sm leading-relaxed text-ink-2">
      <p>
        Built for the{" "}
        <a href={copy.challengeURL} className={external} target="_blank" rel="noopener noreferrer">
          DEV Weekend Challenge: Generosity Edition
        </a>{" "}
        ·{" "}
        <a href={copy.repoURL} className={external} target="_blank" rel="noopener noreferrer">
          Source on GitHub
        </a>{" "}
        · Runs on Solana devnet. A proof-of-concept ledger, not a mainnet contract.
      </p>
    </footer>
  );
}
