// Every sentence a visitor can read, in one place.

export const copy = {
  title: "Kindness Chain",
  tagline: ["One sentence from you.", "One dime from me.", "Verified on-chain."],
  fieldLabel: "One kind thing you did or will do today",
  namePlaceholder: "I carried my neighbour's groceries up three floors.",
  nameLabel: "Your name (optional)",
  button: "Add my link",
  tooShort: "Write at least 10 characters — one honest sentence is enough.",
  tooLong: "Keep it to 200 characters — one sentence is enough.",
  offline: "Could not reach the server. Check your connection and try again.",
  added: (n: number) => `Link #${n} added — confirming on-chain…`,
  confirmed: (n: number) => `Link #${n} confirmed.`,
  verifyOnExplorer: "Verify it on Solana Explorer.",
  failed: (n: number) => `Link #${n} could not be written to the chain. Try again in a minute.`,
  pendingChip: "confirming on-chain…",
  confirmedChip: "confirmed",
  failedChip: "failed on-chain",
  paused: "The chain is paused: Solana devnet is not answering right now. Links wait in line, and nothing is lost.",
  howItWorks: "How it works",
  feedHeading: "The chain",
  feedOrder: "newest first",
  loadOlder: "Load older",
  loading: "Loading the chain…",
  loadFailed: "Could not load the chain. Refresh to try again.",
  verify: "Verify",
  explorer: "Explorer ↗",
  share: "Share",
  copied: "Copied",
  checking: "Reading the memo from the chain…",
  matches: "On-chain memo matches ✓",
  differs: "The memo on the chain differs from the stored link.",
  showMemo: "Show the memo",
  challengeURL: "https://dev.to/challenges/weekend-2026-09-03",
  repoURL: "https://github.com/nazboyko/kindness-chain",
};

export const limits = { minAct: 10, maxAct: 200, maxName: 40 };

// shareOpening is the sentence that goes out with a link's address.
export function shareOpening(n: number, mine: boolean): string {
  const opening = mine ? `I just added link #${n} to the Kindness Chain` : `Link #${n} on the Kindness Chain`;
  return `${opening} — every sentence becomes a dime for refugees in Minnesota. Add yours:`;
}

export function linkAddress(n: number): string {
  return `${window.location.origin}/?link=${n}`;
}
