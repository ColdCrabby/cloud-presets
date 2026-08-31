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
  // The API reference and OpenAPI spec are served by the API itself under /v1.
  // apiBaseUrl is empty (same origin) in dev and prod, so this resolves to
  // /v1/docs; set it only if the API is ever hosted on another origin.
  protected readonly apiDocsUrl = `${environment.apiBaseUrl}/v1/docs`;
}
