/**
 * Public entry point for the generated Cloud Presets API client.
 *
 * Everything under `./generated` is produced by `pnpm gen:client` from
 * `openapi/openapi.json` and committed, so a spec change lands as a reviewable
 * diff. Do not edit generated files by hand.
 */
export * from './generated';
export { client as cloudPresetsClient } from './generated/client.gen';

import { client } from './generated/client.gen';

/**
 * Point the shared client at an API base URL. Apps call this once at startup
 * (e.g. from their environment config). During local development the base URL
 * is left as same-origin so the Angular dev-server proxy forwards `/v1` to the
 * stub API.
 */
export function configureCloudPresetsClient(baseUrl: string): void {
  client.setConfig({ baseUrl });
}
