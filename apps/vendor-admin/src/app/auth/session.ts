import { Injectable, computed, signal } from '@angular/core';
import { StytchUIClient, mountLogin, Products } from '@stytch/vanilla-js';
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
  private sessionJwt: string | null = null;
  private stytch: StytchUIClient | null = null;

  readonly member = this._member.asReadonly();
  readonly isAuthenticated = computed(() => this._member() !== null);
  readonly usesStytch = this.hasRealToken();

  constructor() {
    setCloudPresetsAuthTokenProvider(() => this.sessionJwt);

    if (this.usesStytch) {
      this.stytch = new StytchUIClient(environment.stytchPublicToken);
      this.stytch.session.onChange(() => void this.syncFromStytch());
      void this.syncFromStytch();
    }
  }

  /** Mount the Stytch login UI into host. No-op in stub mode. */
  mountLogin(host: HTMLElement): void {
    if (!this.stytch) {
      return;
    }
    if (!host.id) {
      host.id = 'stytch-login';
    }
    // Consumer login uses the Login/Sign-up redirect URLs registered in the
    // Stytch dashboard — no Discovery type needed.
    const redirectURL = `${globalThis.location.origin}/vendor/`;
    mountLogin({
      client: this.stytch,
      elementId: `#${host.id}`,
      config: {
        products: [Products.emailMagicLinks, Products.oauth],
        emailMagicLinksOptions: {
          loginRedirectURL: redirectURL,
          signupRedirectURL: redirectURL,
        },
        oauthOptions: {
          providers: [{ type: 'google' }, { type: 'github' }],
          loginRedirectURL: redirectURL,
          signupRedirectURL: redirectURL,
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

  private async syncFromStytch(): Promise<void> {
    const tokens = this.stytch?.session.getTokens();
    this.sessionJwt = tokens?.session_jwt ?? null;
    if (!this.sessionJwt) {
      this._member.set(null);
      return;
    }
    try {
      const res = await fetch(`${environment.apiBaseUrl}/v1/me`, {
        headers: { Authorization: `Bearer ${this.sessionJwt}` },
      });
      if (!res.ok) {
        this._member.set(null);
        return;
      }
      const body = (await res.json()) as {
        memberId: string;
        organizationSlug: string;
        roles?: string[];
      };
      this._member.set({
        memberId: body.memberId,
        organizationSlug: body.organizationSlug,
        roles: body.roles ?? [],
      });
    } catch {
      this._member.set(null);
    }
  }
}
