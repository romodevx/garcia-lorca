import { fold } from "./fold";
import { type Command, KIND_ORDER, type MatchOptions, type ScoredCommand } from "./types";

/**
 * Match + rank commands against a query. Pure, platform-agnostic.
 *
 * Ranking (the plan's priority order dominates textual score):
 *   1. kind    — module < create < recent < record < quick  (KIND_ORDER)
 *   2. score   — substring beats subsequence; earlier/prefix beats later
 *   3. input order (stable) — ties keep registry order
 *
 * An empty query returns every command ranked by kind (the palette's resting
 * state). A non-empty query excludes anything with score 0.
 */
const DEFAULT_LIMIT = 8;

const SUBSTRING_BASE = 1000;
const SUBSEQUENCE_SCORE = 100;
const PREFIX_BONUS = 500;

export function matchCommands(
  commands: readonly Command[],
  query: string,
  options: MatchOptions = {},
): ScoredCommand[] {
  const limit = options.limit ?? DEFAULT_LIMIT;
  const needle = fold(query);

  const scored: Array<ScoredCommand & { index: number }> = [];
  commands.forEach((command, index) => {
    const score = needle === "" ? 1 : scoreCommand(command, needle);
    if (score > 0) scored.push({ command, score, index });
  });

  scored.sort((a, b) => {
    const kindDelta = KIND_ORDER[a.command.kind] - KIND_ORDER[b.command.kind];
    if (kindDelta !== 0) return kindDelta;
    if (a.score !== b.score) return b.score - a.score;
    return a.index - b.index;
  });

  return scored.slice(0, limit).map(({ command, score }) => ({ command, score }));
}

/** Best score of the needle across a command's title + aliases (0 = no match). */
function scoreCommand(command: Command, needle: string): number {
  const haystacks = [command.title, ...(command.aliases ?? [])];
  let best = 0;
  for (const haystack of haystacks) {
    best = Math.max(best, scoreText(fold(haystack), needle));
  }
  return best;
}

function scoreText(haystack: string, needle: string): number {
  const at = haystack.indexOf(needle);
  if (at >= 0) {
    // Substring: earlier is better; a prefix match gets an extra bump.
    return SUBSTRING_BASE - at + (at === 0 ? PREFIX_BONUS : 0);
  }
  return isSubsequence(haystack, needle) ? SUBSEQUENCE_SCORE : 0;
}

/** True if `needle` appears in `haystack` as an ordered subsequence. */
function isSubsequence(haystack: string, needle: string): boolean {
  let i = 0;
  for (const char of haystack) {
    if (char === needle[i]) i += 1;
    if (i === needle.length) return true;
  }
  return i === needle.length;
}
