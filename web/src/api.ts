export type Status = "pending" | "confirmed" | "failed";

export interface Link {
  n: number;
  act: string;
  by: string;
  createdAt: string;
  status: Status;
  signature?: string;
  prev?: string;
  confirmedAt?: string;
  explorerUrl?: string;
}

export interface Stats {
  count: number;
  pending: number;
  pledgedCents: number;
  capCents: number;
  perLinkCents: number;
  charity: { name: string; url: string };
  pledger: string;
  cluster: string;
  signer?: { address: string; explorerUrl: string };
  head?: { n: number; signature: string; explorerUrl: string };
  paused: boolean;
}

export interface Memo {
  v: number;
  n: number;
  act: string;
  by: string;
  prev: string;
  t: string;
}

export interface Verification {
  n: number;
  signature: string;
  explorerUrl: string;
  matches: boolean;
  onChain?: Memo;
  stored: Memo;
  raw?: string;
  problem?: string;
}

export interface Page {
  links: Link[];
  hasMore: boolean;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly retryAfterSeconds?: number,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (init?.body) headers["Content-Type"] = "application/json";
  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    let message = `The server answered ${res.status}.`;
    let retryAfterSeconds: number | undefined;
    try {
      const body = await res.json();
      if (typeof body.error === "string") message = body.error;
      if (typeof body.retryAfterSeconds === "number") retryAfterSeconds = body.retryAfterSeconds;
    } catch {
      // no JSON body, the status message stands
    }
    throw new ApiError(res.status, message, retryAfterSeconds);
  }
  return res.json() as Promise<T>;
}

export const api = {
  stats: () => request<Stats>("/api/stats"),
  links: (before?: number, limit = 50) =>
    request<Page>(`/api/links?limit=${limit}${before ? `&before=${before}` : ""}`),
  link: (n: number) => request<Link>(`/api/links/${n}`),
  add: (act: string, by: string, website: string) =>
    request<Link>("/api/links", { method: "POST", body: JSON.stringify({ act, by, website }) }),
  verify: (n: number) => request<Verification>(`/api/verify/${n}`),
};
