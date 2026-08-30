import { Injectable, inject, signal } from '@angular/core';
import { environment } from '../../environments/environment';
import { SessionStore } from '../auth/session';

/** One preset file within an uploaded draft, as returned by the API. */
export interface DraftFile {
  readonly kind: string;
  readonly id: string;
  readonly name: string;
  readonly vendor?: string;
  readonly fileName: string;
  readonly content: string;
}

/** A parked upload the vendor can review and claim. */
export interface Draft {
  readonly id: string;
  readonly createdAt?: string;
  readonly expiresAt?: string;
  readonly files: readonly DraftFile[];
}

/** The outcome of claiming a draft. */
export interface ClaimResult {
  readonly claimed: boolean;
  readonly prCreated: boolean;
  readonly vendor: string;
  readonly files: readonly string[];
  readonly pullRequestUrl?: string;
  readonly branch?: string;
  readonly message?: string;
}

interface ProblemDetails {
  readonly detail?: string;
  readonly title?: string;
  readonly errors?: readonly {
    file: string;
    message?: string;
    errors?: { path: string; message: string }[];
  }[];
}

/**
 * Client for the manual upload / claim endpoints.
 *
 * These live outside the generated OpenAPI client because they deal in
 * multipart bodies and a hand-off id, so they are called directly with fetch —
 * the same approach the session uses for `/v1/me`.
 */
@Injectable({ providedIn: 'root' })
export class UploadService {
  private readonly session = inject(SessionStore);
  private readonly base = environment.apiBaseUrl;

  private readonly _busy = signal(false);
  readonly busy = this._busy.asReadonly();

  /** POST preset files (or a .zip). `kind` labels bare files whose name does
   *  not encode a type; it is ignored for a .zip, which is read by layout. */
  async upload(
    files: FileList | File[],
    kind?: 'printer' | 'filament',
  ): Promise<{ id: string; claimUrl: string; files: DraftFile[] }> {
    const form = new FormData();
    for (const file of Array.from(files)) {
      form.append(this.fieldFor(file, kind), file, file.name);
    }
    this._busy.set(true);
    try {
      const res = await fetch(`${this.base}/v1/uploads`, { method: 'POST', body: form });
      const body = await this.parse(res);
      if (!res.ok) {
        throw new Error(this.problemMessage(body));
      }
      return body as { id: string; claimUrl: string; files: DraftFile[] };
    } finally {
      this._busy.set(false);
    }
  }

  /** GET a parked draft by id so it can be reviewed before claiming. */
  async getDraft(id: string): Promise<Draft> {
    const res = await fetch(`${this.base}/v1/uploads/${encodeURIComponent(id)}`);
    const body = await this.parse(res);
    if (!res.ok) {
      throw new Error(this.problemMessage(body));
    }
    return body as Draft;
  }

  /** Claim a draft: authorize against the caller's vendor and open a PR. */
  async claim(id: string, vendor?: string): Promise<ClaimResult> {
    const token = this.session.authToken();
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    this._busy.set(true);
    try {
      const res = await fetch(`${this.base}/v1/uploads/${encodeURIComponent(id)}/claim`, {
        method: 'POST',
        headers,
        body: JSON.stringify(vendor ? { vendor } : {}),
      });
      const body = await this.parse(res);
      if (!res.ok) {
        throw new Error(this.problemMessage(body));
      }
      return body as ClaimResult;
    } finally {
      this._busy.set(false);
    }
  }

  private fieldFor(file: File, kind?: 'printer' | 'filament'): string {
    const name = file.name.toLowerCase();
    if (name.endsWith('.zip')) return 'archive';
    // A field name matching a preset kind tells the API how to validate a bare
    // file; a printers/ or filaments/ path would do the same, but plain file
    // names carry no layout, so the selected kind is used.
    return kind ?? 'preset';
  }

  private async parse(res: Response): Promise<unknown> {
    const text = await res.text();
    if (!text) return {};
    try {
      return JSON.parse(text);
    } catch {
      return { detail: text };
    }
  }

  private problemMessage(body: unknown): string {
    const p = body as ProblemDetails;
    if (p?.errors?.length) {
      return p.errors
        .map((e) => {
          const inner = e.errors?.map((x) => `${x.path}: ${x.message}`).join('; ');
          return inner ? `${e.file} — ${inner}` : `${e.file}${e.message ? ` — ${e.message}` : ''}`;
        })
        .join('\n');
    }
    return p?.detail ?? p?.title ?? 'The request failed. Please try again.';
  }
}
