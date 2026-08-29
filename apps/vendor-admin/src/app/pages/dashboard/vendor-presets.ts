import { Injectable, signal } from '@angular/core';
import { listVendorPresets, type PresetSummary } from '@cloud-presets/api-client';

/** Signal-based wrapper over the caller-scoped vendor presets endpoint. */
@Injectable({ providedIn: 'root' })
export class VendorPresets {
  private readonly _items = signal<PresetSummary[]>([]);
  private readonly _loading = signal(false);
  private readonly _error = signal<string | null>(null);

  readonly items = this._items.asReadonly();
  readonly loading = this._loading.asReadonly();
  readonly error = this._error.asReadonly();

  async load(): Promise<void> {
    this._loading.set(true);
    this._error.set(null);
    const { data, error } = await listVendorPresets();
    if (error) {
      this._error.set('Could not load your presets. Is the stub API running?');
      this._items.set([]);
    } else {
      this._items.set(data ?? []);
    }
    this._loading.set(false);
  }
}
