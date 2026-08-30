import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { ThemeToggle } from '../theme-toggle/theme-toggle';

/**
 * Web-focused application shell shared by every Cold Crabby frontend.
 *
 * A sticky, frosted top bar (brand · section on the left, projected nav plus
 * the light/dark switcher on the right) over a centred, max-width content
 * column. It mirrors the slicer's design language — same tokens, same accent —
 * but in a scrolling web layout rather than the desktop app's titlebar + rail.
 *
 * ```html
 * <ccc-app-shell section="Preset Catalog">
 *   <a shell-nav [href]="vendorUrl">Vendor login</a>
 *   <!-- routed page content -->
 * </ccc-app-shell>
 * ```
 */
@Component({
  selector: 'ccc-app-shell',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ThemeToggle],
  templateUrl: './app-shell.html',
  styleUrl: './app-shell.scss',
})
export class AppShell {
  /** Section label shown next to the wordmark (e.g. "Preset Catalog"). */
  readonly section = input('');
  /** Optional href the wordmark links to; omit to render it as plain text. */
  readonly homeUrl = input<string>();
}
