import { Injectable, signal } from '@angular/core';
import {
  uploadPresets,
  getUpload,
  claimUpload,
  type DraftFileView,
  type UploadOutputBody,
  type GetUploadOutputBody,
  type ClaimOutputBody,
} from '@cloud-presets/api-client';

export type { DraftFileView };
export type ClaimResult = ClaimOutputBody;

/** A parked upload the vendor can review and claim. */
export type Draft = GetUploadOutputBody;

interface ProblemDetails {
  readonly detail?: string;
  readonly title?: string;
  readonly errors?: readonly { location?: string; message?: string }[];
}

/**
 * Client for the manual upload / claim endpoints.
 *
 * These are first-class operations in the generated OpenAPI client, so every
 * call goes through the shared client — the session JWT is attached by the
 * client's auth interceptor, and the base URL is the one configured at startup.
 * The service only adapts the typed results into signals and readable errors.
 */
@Injectable({ providedIn: 'root' })
export class UploadService {
  private readonly _busy = signal(false);
  readonly busy = this._busy.asReadonly();

  /** POST preset files (or a .zip). `kind` labels bare files whose name does
   *  not encode a type; it is ignored for a .zip, which is read by layout. */
  async upload(files: FileList | File[], kind?: 'printer' | 'filament'): Promise<UploadOutputBody> {
    this._busy.set(true);
    try {
      const { data, error } = await uploadPresets({
        body: { files: Array.from(files) },
        query: kind ? { type: kind } : undefined,
      });
      if (error || !data) {
        throw new Error(this.problemMessage(error));
      }
      return data;
    } finally {
      this._busy.set(false);
    }
  }

  /** GET a parked draft by id so it can be reviewed before claiming. */
  async getDraft(id: string): Promise<Draft> {
    const { data, error } = await getUpload({ path: { id } });
    if (error || !data) {
      throw new Error(this.problemMessage(error));
    }
    return data;
  }

  /** Claim a draft: authorize against the caller's vendor and open a PR. */
  async claim(id: string, vendor?: string): Promise<ClaimOutputBody> {
    this._busy.set(true);
    try {
      const { data, error } = await claimUpload({
        path: { id },
        body: vendor ? { vendor } : {},
      });
      if (error || !data) {
        throw new Error(this.problemMessage(error));
      }
      return data;
    } finally {
      this._busy.set(false);
    }
  }

  private problemMessage(error: unknown): string {
    const p = error as ProblemDetails;
    if (p?.errors?.length) {
      return p.errors
        .map((e) => (e.location ? `${e.location} — ${e.message ?? ''}` : (e.message ?? '')))
        .join('\n');
    }
    return p?.detail ?? p?.title ?? 'The request failed. Please try again.';
  }
}
