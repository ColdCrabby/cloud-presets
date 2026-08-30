import {
  ChangeDetectionStrategy,
  Component,
  type ElementRef,
  effect,
  inject,
  viewChild,
} from '@angular/core';
import { Button } from '@coldcrabby/ui';
import { Card } from '@cloud-presets/ui';
import { SessionStore } from '../../auth/session';
import { UploadPanel } from '../../uploads/upload-panel';
import { VendorPresets } from './vendor-presets';

@Component({
  selector: 'ccc-dashboard',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [Button, Card, UploadPanel],
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
