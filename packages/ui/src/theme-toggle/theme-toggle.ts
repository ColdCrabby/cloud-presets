import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { Icon, IconButton, ThemeService, TooltipDirective } from '@coldcrabby/ui';

/**
 * Compact light/dark switcher for a web app's header. Shows the active scheme
 * (sun in dark mode, moon in light mode) and flips it on click via the shared
 * {@link ThemeService}. Icon-only, so it carries an `aria-label` and tooltip.
 *
 * Web-only: the desktop slicer drives its colour scheme from the OS shell, so
 * this lives with the web frontends rather than in the shared UI library.
 *
 * ```html
 * <ccc-theme-toggle />
 * ```
 */
@Component({
  selector: 'ccc-theme-toggle',
  standalone: true,
  imports: [IconButton, Icon, TooltipDirective],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <button
      nexusIconButton
      type="button"
      [attr.aria-label]="label()"
      [attr.aria-pressed]="theme.isDarkMode()"
      [tooltip]="label()"
      (click)="theme.toggleTheme()"
    >
      <nexus-icon [name]="theme.isDarkMode() ? 'sun-light' : 'half-moon'" />
    </button>
  `,
})
export class ThemeToggle {
  protected readonly theme = inject(ThemeService);

  protected readonly label = computed(() =>
    this.theme.isDarkMode() ? 'Switch to light theme' : 'Switch to dark theme',
  );
}
