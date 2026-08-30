import { Injectable, signal } from '@angular/core';
import { type PresetSummary, searchPresets } from '@cloud-presets/api-client';

/**
 * Thin signal-based wrapper over the generated search endpoint. Components read
 * the signals; the app never hand-writes request/response types.
 */
@Injectable({ providedIn: 'root' })
export class Catalog {
  private readonly _results = signal<PresetSummary[]>([]);
  private readonly _loading = signal(false);
  private readonly _error = signal<string | null>(null);
  private readonly _revision = signal<string | null>(null);

  readonly results = this._results.asReadonly();
  readonly loading = this._loading.asReadonly();
  readonly error = this._error.asReadonly();
  readonly revision = this._revision.asReadonly();

  async search(query: string): Promise<void> {
    this._loading.set(true);
    this._error.set(null);
    const q = query.trim();
    const { data, error } = await searchPresets({ query: q ? { q } : {} });
    if (error) {
      const detail = (error as { detail?: string }).detail;
      this._error.set(detail ?? 'Search is temporarily unavailable. Please try again.');
      this._results.set([]);
    } else {
      this._results.set(data?.results ?? []);
      this._revision.set(data?.revision ?? null);
    }
    this._loading.set(false);
  }
}
