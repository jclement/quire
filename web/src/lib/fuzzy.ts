// Fuzzy matching for the command palette and list filters. Deliberately small:
// case-insensitive subsequence match with bonuses that make exact titles rank
// first, then prefixes, then word-boundary matches — the ordering the palette
// spec requires. No external dependency; scores only need to be comparable
// within one query.

/** Score for a query exactly equal to the target (highest possible). */
const EXACT_SCORE = 10_000;
/** Base score when the target starts with the query. */
const PREFIX_SCORE = 5_000;
/** Per-character bonus for matching at a word boundary. */
const BOUNDARY_BONUS = 10;
/** Per-character bonus for matching adjacent to the previous match. */
const CONSECUTIVE_BONUS = 5;

/**
 * Scores `query` against `target`. Higher is better; -1 means no match.
 * An empty query matches everything with score 0.
 */
export function fuzzyScore(query: string, target: string): number {
  const q = query.toLowerCase();
  const t = target.toLowerCase();
  if (q.length === 0) return 0;
  if (q === t) return EXACT_SCORE;
  if (t.startsWith(q)) return PREFIX_SCORE - t.length;

  let score = 0;
  let tIndex = 0;
  let lastMatch = -2;
  for (const char of q) {
    const found = t.indexOf(char, tIndex);
    if (found === -1) return -1;
    if (found === 0 || !isWordChar(t[found - 1] ?? " "))
      score += BOUNDARY_BONUS;
    if (found === lastMatch + 1) score += CONSECUTIVE_BONUS;
    score += 1;
    lastMatch = found;
    tIndex = found + 1;
  }
  // Prefer shorter targets when the match quality is otherwise equal.
  return score * 100 - t.length;
}

function isWordChar(char: string): boolean {
  return /[a-z0-9]/i.test(char);
}

/**
 * Filters and sorts `items` by fuzzy score against `query`, best first.
 * Stable for equal scores, so callers control tie-break order via input order.
 */
export function fuzzyRank<T>(
  query: string,
  items: T[],
  getText: (item: T) => string,
): T[] {
  const scored = items
    .map((item, order) => ({
      item,
      order,
      score: fuzzyScore(query, getText(item)),
    }))
    .filter((entry) => entry.score >= 0);
  scored.sort((a, b) => b.score - a.score || a.order - b.order);
  return scored.map((entry) => entry.item);
}
