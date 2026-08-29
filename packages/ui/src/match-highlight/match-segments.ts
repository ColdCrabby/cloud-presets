/** A contiguous slice of a field value, flagged as a query match or not. */
export interface HighlightSegment {
  readonly text: string;
  readonly matched: boolean;
}

/** A [start, end) pair of UTF-16 code-unit offsets, as returned by the API. */
export type MatchRange = readonly [number, number];

/**
 * Split `value` into matched / unmatched segments from the API's match ranges.
 *
 * Ranges are UTF-16 code-unit offsets into the original, unnormalised value —
 * the same coordinate system JavaScript strings use — so slicing with them
 * directly is correct. Ranges are clamped and merged defensively so a
 * malformed or overlapping range can never throw or drop characters.
 */
export function matchSegments(value: string, ranges: readonly MatchRange[]): HighlightSegment[] {
  const length = value.length;
  const clamped = ranges
    .map(([start, end]): MatchRange => [
      Math.max(0, Math.min(start, length)),
      Math.max(0, Math.min(end, length)),
    ])
    .filter(([start, end]) => end > start)
    .sort((a, b) => a[0] - b[0]);

  const merged: Array<[number, number]> = [];
  for (const [start, end] of clamped) {
    const last = merged[merged.length - 1];
    if (last && start <= last[1]) {
      last[1] = Math.max(last[1], end);
    } else {
      merged.push([start, end]);
    }
  }

  if (merged.length === 0) {
    return value.length ? [{ text: value, matched: false }] : [];
  }

  const segments: HighlightSegment[] = [];
  let cursor = 0;
  for (const [start, end] of merged) {
    if (start > cursor) {
      segments.push({ text: value.slice(cursor, start), matched: false });
    }
    segments.push({ text: value.slice(start, end), matched: true });
    cursor = end;
  }
  if (cursor < length) {
    segments.push({ text: value.slice(cursor), matched: false });
  }
  return segments;
}
