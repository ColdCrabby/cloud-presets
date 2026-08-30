import {
  type ApplicationConfig,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { provideMarkdown } from 'ngx-markdown';
import { configureCloudPresetsClient } from '@cloud-presets/api-client';
import { environment } from '../environments/environment';
import { routes } from './app.routes';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),
    // Runtime peers for @coldcrabby/ui: Icon fetches its SVGs over HTTP, and
    // tooltips can render markdown in block mode.
    provideHttpClient(),
    provideMarkdown(),
    provideAppInitializer(() => {
      configureCloudPresetsClient(environment.apiBaseUrl);
    }),
  ],
};
