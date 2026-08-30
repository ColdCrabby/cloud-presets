import {
  ChangeDetectionStrategy,
  Component,
  type ElementRef,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Button } from '@coldcrabby/ui';
import { Card } from '@cloud-presets/ui';
import { SessionStore } from '../../auth/session';
import { UploadService, type ClaimResult, type Draft } from '../../uploads/upload.service';

/**
 * Claim page: load an uploaded draft by id, show its files, and — once the
 * vendor is signed in — open a pull request for it.
 *
 * Loading the draft needs no session (the id is the capability). Claiming does:
 * the API authorizes the caller's organization against the vendor manifest and
 * opens the PR under the bot. So an unauthenticated visitor sees the review plus
 * the sign-in widget, exactly like the slicer hand-off.
 */
@Component({
  selector: 'ccc-claim',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [Button, Card, RouterLink],
  templateUrl: './claim.html',
  styleUrl: './claim.scss',
})
export class Claim {
  protected readonly session = inject(SessionStore);
  private readonly uploads = inject(UploadService);
  private readonly route = inject(ActivatedRoute);

  protected readonly draft = signal<Draft | null>(null);
  protected readonly loadError = signal<string | null>(null);
  protected readonly claimError = signal<string | null>(null);
  protected readonly result = signal<ClaimResult | null>(null);
  protected readonly busy = this.uploads.busy;

  private readonly loginHost = viewChild<ElementRef<HTMLElement>>('loginHost');
  private id = '';

  constructor() {
    this.id = this.route.snapshot.paramMap.get('id') ?? '';
    void this.load();

    // Mount the Stytch login widget for an unauthenticated visitor.
    effect(() => {
      const host = this.loginHost()?.nativeElement;
      if (host && !this.session.isAuthenticated() && this.session.usesStytch) {
        this.session.mountLogin(host);
      }
    });
  }

  private async load(): Promise<void> {
    if (!this.id) {
      this.loadError.set('This claim link is missing an upload id.');
      return;
    }
    try {
      this.draft.set(await this.uploads.getDraft(this.id));
    } catch (e) {
      this.loadError.set(e instanceof Error ? e.message : String(e));
    }
  }

  protected async claim(): Promise<void> {
    this.claimError.set(null);
    try {
      this.result.set(await this.uploads.claim(this.id));
    } catch (e) {
      this.claimError.set(e instanceof Error ? e.message : String(e));
    }
  }
}
