import { ChangeDetectionStrategy, Component, input } from '@angular/core';

/**
 * Minimal shared button primitive. Both apps use it so their buttons share one
 * house style rather than each re-implementing the same variants.
 */
@Component({
  selector: 'ccc-button',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <button
      class="ccc-button"
      [class.ccc-button--primary]="variant() === 'primary'"
      [class.ccc-button--ghost]="variant() === 'ghost'"
      [attr.type]="type()"
      [disabled]="disabled()"
    >
      <ng-content />
    </button>
  `,
  styleUrl: './button.scss',
})
export class CccButton {
  readonly variant = input<'primary' | 'ghost'>('primary');
  readonly type = input<'button' | 'submit'>('button');
  readonly disabled = input(false);
}
