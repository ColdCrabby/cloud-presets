import { Injectable, computed, signal } from '@angular/core';
import { createStytchB2BClient, StytchB2B, B2BProducts } from '@stytch/vanilla-js/b2b';
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
 * When a real Stytch B2B public token is configured it drives the prebuilt
 * `StytchB2B` login element (OAuth + email magic link, discovery flow) and,
 * once a session JWT exists, calls the API's `/v1/me` to resolve the member.
 * With no token configured it falls back to a stub so the UI still runs.
 */
@Injectable({ providedIn: 'root' })
export class SessionStore {
  private readonly _member = signal<Member | null>(null);
  private sessionJwt: string | null = null;
  private stytch: ReturnType<typeof createStytchB2BClient> | null = null;

  readonly member = this._member.asReadonly();
  readonly isAuthenticated = computed(() => this._member() !== null);
  readonly usesStytch = this.hasRealToken();

  constructor() {
    setCloudPresetsAuthTokenProvider(() => this.sessionJwt);

    if (this.usesStytch) {
      this.stytch = createStytchB2BClient(environment.stytchPublicToken);
      if (!customElements.get('stytch-b2b')) {
        customElements.define('stytch-b2b', StytchB2B);
      }
      this.stytch.session.onChange(() => void this.syncFromStytch());
      void this.syncFromStytch();
    }
  }

  /** Mount the Stytch login element into host. No-op in stub mode. */
  mountLogin(host: HTMLElement): void {
    if (!this.stytch) {
      return;
    }
    const el = document.createElement('stytch-b2b') as HTMLElement & {
      render: (opts: unknown) => void;
    };
    host.replaceChildren(el);
    // Pin the discovery redirect to this app's /vendor/ URL so it matches the
    // URL registered in the Stytch dashboard (Discovery redirect type).
    const redirectURL = `${globalThis.location.origin}/vendor/`;
    el.render({
      client: this.stytch,
      config: {
        authFlowType: 'Discovery',
        products: [B2BProducts.emailMagicLinks, B2BProducts.oauth],
        oauthOptions: {
          providers: [{ type: 'github' }, { type: 'google' }],
          discoveryRedirectURL: redirectURL,
        },
        emailMagicLinksOptions: { discoveryRedirectURL: redirectURL },
        sessionOptions: { sessionDurationMinutes: 60 },
      },
    });
  }

  /** Stub sign-in for when Stytch is not configured. */
  signIn(): void {
    this._member.set({
      memberId: 'member-stub',
      organizationSlug: 'prusa-research',
      roles: ['stytch_member'],
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
