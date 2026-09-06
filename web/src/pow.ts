import { api, type Seal } from "./api";

// seal asks the server for a challenge and finds a nonce so that the
// SHA-256 of seed + nonce + sentence starts with enough zero bits. A
// phone takes a second or two; a script that wants thousands of links
// pays that thousands of times. With the difficulty at zero the server
// is not asking, and nothing is sent.
export async function seal(act: string): Promise<Seal | undefined> {
  const challenge = await api.challenge();
  if (challenge.difficulty <= 0) return undefined;
  const nonce = await solve(challenge.seed, challenge.difficulty, act);
  return { seed: challenge.seed, nonce: String(nonce) };
}

const batch = 256;

async function solve(seed: string, bits: number, act: string): Promise<number> {
  const encoder = new TextEncoder();
  for (let start = 0; ; start += batch) {
    // digests are issued in batches so the browser can pipeline them and
    // the page stays responsive between batches
    const digests = await Promise.all(
      Array.from({ length: batch }, (_, i) => crypto.subtle.digest("SHA-256", encoder.encode(seed + (start + i) + act))),
    );
    for (let i = 0; i < batch; i++) {
      if (leadingZeroBits(new Uint8Array(digests[i])) >= bits) return start + i;
    }
  }
}

function leadingZeroBits(bytes: Uint8Array): number {
  let n = 0;
  for (const b of bytes) {
    if (b === 0) {
      n += 8;
      continue;
    }
    n += Math.clz32(b) - 24;
    break;
  }
  return n;
}
