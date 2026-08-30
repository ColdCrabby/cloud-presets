import { ChangeDetectionStrategy, Component } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { AppShell } from '@cloud-presets/ui';
import { environment } from '../environments/environment';

@Component({
  selector: 'ccc-root',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterOutlet, AppShell],
  templateUrl: './app.html',
  styleUrl: './app.scss',
})
export class App {
  protected readonly vendorUrl = environment.vendorUrl;
}
