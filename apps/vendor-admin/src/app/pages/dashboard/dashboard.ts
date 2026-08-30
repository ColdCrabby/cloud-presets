import {
  ChangeDetectionStrategy,
  Component,
  type ElementRef,
  effect,
  inject,
  viewChild,
} from '@angular/core';
import { CccButton, CccCard } from '@cloud-presets/ui';
import { SessionStore } from '../../auth/session';
import { VendorPresets } from './vendor-presets';

@Component({
  selector: 'ccc-dashboard',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CccButton, CccCard],
  templateUrl: './dashboard.html',
  styleUrl: './dashboard.scss',
})
export class Dashboard {
  protected readonly session = inject(SessionStore);
  protected readonly presets = inject(VendorPresets);

  private readonly loginHost = viewChild<ElementRef<HTMLElement>>('loginHost');

  constructor() {
    // Load the caller-scoped presets whenever a session becomes active.
    effect(() => {
      if (this.session.isAuthenticated()) {
        void this.presets.load();
      }
    });

    // Mount the Stytch login element once its host exists and we're signed out.
    effect(() => {
      const host = this.loginHost()?.nativeElement;
      if (host && !this.session.isAuthenticated() && this.session.usesStytch) {
        this.session.mountLogin(host);
      }
    });
  }
}
