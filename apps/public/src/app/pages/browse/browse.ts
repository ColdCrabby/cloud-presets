import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { Button } from '@coldcrabby/ui';
import { Card, MatchHighlight } from '@cloud-presets/ui';
import { Catalog } from '../../catalog';

@Component({
  selector: 'ccc-browse',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [Button, Card, MatchHighlight],
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
