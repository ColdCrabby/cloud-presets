import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { CccButton, CccCard, CccMatchHighlight } from '@cloud-presets/ui';
import { Catalog } from '../../catalog';

@Component({
  selector: 'ccc-browse',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CccButton, CccCard, CccMatchHighlight],
  templateUrl: './browse.html',
  styleUrl: './browse.scss',
})
export class Browse {
  protected readonly catalog = inject(Catalog);
  protected readonly query = signal('');

  constructor() {
    void this.catalog.search('');
  }

  protected onInput(event: Event): void {
    this.query.set((event.target as HTMLInputElement).value);
  }

  protected submit(): void {
    void this.catalog.search(this.query());
  }
}
