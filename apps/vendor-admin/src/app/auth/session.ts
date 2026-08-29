import { Injectable, computed, signal } from '@angular/core';

export interface Member {
  readonly memberId: string;
  readonly email: string;
  readonly organizationName: string;
}

/**
 * Session seam for the vendor-admin app.
 *
 * There is no official Angular SDK for Stytch — only React — so this is the
 * thin wrapper the architecture calls for: it exposes the session as Angular
 * signals so components stay framework-idiomatic. The auth issue replaces the
 * stubbed sign-in below with `@stytch/vanilla-js` (B2B), including the prebuilt
 * Admin Portal components; the signal surface it presents here stays the same.
 */
@Injectable({ providedIn: 'root' })
export class SessionStore {
  private readonly _member = signal<Member | null>(null);

  readonly member = this._member.asReadonly();
  readonly isAuthenticated = computed(() => this._member() !== null);

  /** Stubbed sign-in. Replaced by a real Stytch session in the auth issue. */
  signIn(): void {
    this._member.set({
      memberId: 'member-stub',
      email: 'vendor@prusa.example',
      organizationName: 'Prusa Research',
    });
  }

  signOut(): void {
    this._member.set(null);
  }
}
