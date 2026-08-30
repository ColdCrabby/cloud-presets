import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { matchSegments, type MatchRange } from './match-segments';

/**
 * Renders a field value with the API-supplied match ranges highlighted. Both
 * apps reuse this so search-result highlighting is identical and reflects the
 * ranking the server actually applied.
 */
@Component({
  selector: 'ccc-match-highlight',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `@for (segment of segments(); track $index) {
    @if (segment.matched) {
      <mark class="highlight">{{ segment.text }}</mark>
    } @else {
      <span>{{ segment.text }}</span>
    }
  }`,
  styleUrl: './match-highlight.scss',
})
export class MatchHighlight {
  readonly value = input.required<string>();
  readonly ranges = input<readonly MatchRange[]>([]);

  protected readonly segments = computed(() => matchSegments(this.value(), this.ranges()));
}
