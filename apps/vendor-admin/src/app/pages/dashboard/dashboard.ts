import { ChangeDetectionStrategy, Component, effect, inject } from '@angular/core';
import { CpButton, CpCard } from '@cloud-presets/ui';
import { SessionStore } from '../../auth/session';
import { VendorPresets } from './vendor-presets';

@Component({
  selector: 'cp-dashboard',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CpButton, CpCard],
  templateUrl: './dashboard.html',
  styleUrl: './dashboard.scss',
})
export class Dashboard {
  protected readonly session = inject(SessionStore);
  protected readonly presets = inject(VendorPresets);

  constructor() {
    // Load the caller-scoped presets whenever a session becomes active.
    effect(() => {
      if (this.session.isAuthenticated()) {
        void this.presets.load();
      }
    });
  }
}
