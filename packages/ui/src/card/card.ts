import { ChangeDetectionStrategy, Component, input } from '@angular/core';

/** Simple surface container shared by both apps. */
@Component({
  selector: 'ccc-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (heading()) {
      <h3 class="ccc-card__heading">{{ heading() }}</h3>
    }
    <div class="ccc-card__body">
      <ng-content />
    </div>
  `,
  styleUrl: './card.scss',
})
export class CccCard {
  readonly heading = input<string>();
}
