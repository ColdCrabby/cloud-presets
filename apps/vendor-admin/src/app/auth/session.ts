import { Injectable, computed, signal } from '@angular/core';
import { StytchUIClient, StytchUI, Products, OTPMethods } from '@stytch/vanilla-js';
import { setCloudPresetsAuthTokenProvider } from '@cloud-presets/api-client';
import { environment } from '../../environments/environment';

export interface Member {
  readonly memberId: string;
  readonly organizationSlug: string;
  readonly roles: readonly string[];
}

/**
 * Session seam for the vendor-admin app.
 *
 * When a real Stytch public token is configured it drives the prebuilt Consumer
 * login UI (email magic link + OAuth) and, once a session JWT exists, calls the
 * API's `/v1/me` to resolve the caller. With no token configured it falls back
 * to a stub so the UI still runs.
 */
@Injectable({ providedIn: 'root' })
export class SessionStore {
  private readonly _member = signal<Member | null>(null);
  private readonly _authError = signal<string | null>(null);
  private sessionJwt: string | null = null;
  private stytch: StytchUIClient | null = null;
  private mounted = false;

  readonly member = this._member.asReadonly();
  readonly authError = this._authError.asReadonly();
  readonly isAuthenticated = computed(() => this._member() !== null);
  readonly usesStytch = this.hasRealToken();

  constructor() {
    setCloudPresetsAuthTokenProvider(() => this.sessionJwt);

    if (this.usesStytch) {
      this.stytch = new StytchUIClient(environment.stytchPublicToken);
      this.stytch.session.onChange(() => void this.syncFromStytch());
      // Handle a magic-link/OAuth redirect ourselves so the single-use token is
      // authenticated exactly once (the UI component would race us for it).
      if (this.callbackToken()) {
        void this.handleCallback();
      } else {
        void this.syncFromStytch();
      }
    }
  }

  /** Mount the Stytch login UI into host. No-op in stub mode or during callback. */
  mountLogin(host: HTMLElement): void {
    if (!this.stytch || this.mounted || this.callbackToken()) {
      return;
    }
    this.mounted = true;
    if (!customElements.get('stytch-login')) {
      customElements.define('stytch-login', StytchUI);
    }
    const el = document.createElement('stytch-login') as HTMLElement & {
      render: (opts: unknown) => void;
    };
    host.replaceChildren(el);
    // Email one-time passcode: the user enters a code on this page, so there is
    // no redirect and no redirect-URL configuration to get wrong.
    el.render({
      client: this.stytch,
      config: {
        products: [Products.otp],
        otpOptions: {
          methods: [OTPMethods.Email],
          expirationMinutes: 10,
        },
        sessionOptions: { sessionDurationMinutes: 60 },
      },
    });
  }

  /** Stub sign-in for when Stytch is not configured. */
  signIn(): void {
    this._member.set({
      memberId: 'member-stub',
      organizationSlug: 'demo',
      roles: [],
    });
  }

  signOut(): void {
    if (this.stytch) {
      void this.stytch.session.revoke();
    }
    this.sessionJwt = null;
    this._member.set(null);
  }

  private hasRealToken(): boolean {
    const t = environment.stytchPublicToken;
    return !!t && t.startsWith('public-token-');
  }

  /** The Stytch token from a magic-link/OAuth redirect, if this is a callback. */
  private callbackToken(): { token: string; type: string } | null {
    const p = new URL(globalThis.location.href).searchParams;
    const token = p.get('token');
    const type = p.get('stytch_token_type');
    return token && type ? { token, type } : null;
  }

  /** Authenticate a redirect token exactly once and surface the real error. */
  private async handleCallback(): Promise<void> {
    const cb = this.callbackToken();
    if (!this.stytch || !cb) {
      return;
    }
    try {
      if (cb.type === 'oauth') {
        await this.stytch.oauth.authenticate(cb.token, { session_duration_minutes: 60 });
      } else {
        await this.stytch.magicLinks.authenticate(cb.token, { session_duration_minutes: 60 });
      }
      this.clearAuthQuery();
      await this.syncFromStytch();
    } catch (e) {
      this.clearAuthQuery();
      const msg = e instanceof Error ? e.message : String(e);
      console.error('stytch callback authenticate failed:', e);
      this._authError.set(msg);
    }
  }

  private async syncFromStytch(): Promise<void> {
    const tokens = this.stytch?.session.getTokens();
    this.sessionJwt = tokens?.session_jwt ?? null;
    if (!this.sessionJwt) {
      this._member.set(null);
      return;
    }

    // A live Stytch session means the user is logged in. Strip the single-use
    // magic-link token from the URL so it can't be reprocessed into an error.
    this.clearAuthQuery();

    // Optimistically authenticated; enrich from the API when it's reachable.
    let member: Member = { memberId: 'me', organizationSlug: '', roles: [] };
    try {
      const res = await fetch(`${environment.apiBaseUrl}/v1/me`, {
        headers: { Authorization: `Bearer ${this.sessionJwt}` },
      });
      if (res.ok) {
        const body = (await res.json()) as {
          memberId: string;
          organizationSlug?: string;
          roles?: string[];
        };
        member = {
          memberId: body.memberId,
          organizationSlug: body.organizationSlug ?? '',
          roles: body.roles ?? [],
        };
      }
    } catch {
      // Keep the optimistic member; a session already exists.
    }
    this._member.set(member);
  }

  private clearAuthQuery(): void {
    const url = new URL(globalThis.location.href);
    if (url.searchParams.has('token') || url.searchParams.has('stytch_token_type')) {
      url.searchParams.delete('token');
      url.searchParams.delete('stytch_token_type');
      globalThis.history.replaceState({}, '', url.toString());
    }
  }
}
