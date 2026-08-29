import { ChangeDetectionStrategy, Component, input } from '@angular/core';

/** Simple surface container shared by both apps. */
@Component({
  selector: 'cp-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (heading()) {
      <h3 class="cp-card__heading">{{ heading() }}</h3>
    }
    <div class="cp-card__body">
      <ng-content />
    </div>
  `,
  styleUrl: './card.scss',
})
export class CpCard {
  readonly heading = input<string>();
}
