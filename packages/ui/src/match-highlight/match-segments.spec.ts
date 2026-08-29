import { matchSegments } from './match-segments';

describe('matchSegments', () => {
  it('returns a single unmatched segment when there are no ranges', () => {
    expect(matchSegments('Prusa MK4', [])).toEqual([{ text: 'Prusa MK4', matched: false }]);
  });

  it('splits a value around a single match range', () => {
    expect(matchSegments('Prusa MK4', [[6, 9]])).toEqual([
      { text: 'Prusa ', matched: false },
      { text: 'MK4', matched: true },
    ]);
  });

  it('merges overlapping and adjacent ranges', () => {
    expect(
      matchSegments('filament', [
        [0, 4],
        [3, 8],
      ]),
    ).toEqual([{ text: 'filament', matched: true }]);
  });

  it('clamps out-of-bounds ranges instead of throwing', () => {
    expect(matchSegments('PLA', [[1, 99]])).toEqual([
      { text: 'P', matched: false },
      { text: 'LA', matched: true },
    ]);
  });

  it('uses UTF-16 offsets so non-ASCII values highlight correctly', () => {
    // "Añjš" — the match covers "ñj" at code-unit offsets 1..3.
    expect(matchSegments('Añjš', [[1, 3]])).toEqual([
      { text: 'A', matched: false },
      { text: 'ñj', matched: true },
      { text: 'š', matched: false },
    ]);
  });
});
