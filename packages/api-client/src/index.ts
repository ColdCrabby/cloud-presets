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
import type { PresetSummary } from './generated';

/**
 * Point the shared client at an API base URL. Apps call this once at startup
 * (e.g. from their environment config). During local development the base URL
 * is left as same-origin so the Angular dev-server proxy forwards `/v1` to the
 * sample API.
 */
export function configureCloudPresetsClient(baseUrl: string): void {
  client.setConfig({ baseUrl });
}

/**
 * A function returning the current session JWT, or null when signed out.
 */
export type AuthTokenProvider = () => string | null;

let authTokenProvider: AuthTokenProvider | null = null;

/**
 * Register the source of the caller's session JWT. Once set, every request the
 * shared client makes carries `Authorization: Bearer <jwt>` when a token is
 * available. Apps call this once at startup.
 */
export function setCloudPresetsAuthTokenProvider(provider: AuthTokenProvider): void {
  authTokenProvider = provider;
}

client.interceptors.request.use((request) => {
  const token = authTokenProvider?.();
  if (token) {
    request.headers.set('Authorization', `Bearer ${token}`);
  }
  return request;
});

/**
 * List the caller's vendor-scoped presets (`GET /v1/vendor/presets`).
 *
 * This endpoint is documented in the API surface but not yet served by the Go
 * backend, so it is intentionally absent from the generated SDK (which is
 * generated from the implemented spec). This thin wrapper over the shared client
 * preserves the call site — with the same base URL and auth interceptor as every
 * generated call — until the backend ships it and the SDK regenerates. It returns
 * the same `{ data, error }` shape as a generated operation.
 */
export function listVendorPresets() {
  return client.get<{ 200: Array<PresetSummary> }, unknown, false>({
    security: [{ scheme: 'bearer', type: 'http' }],
    url: '/v1/vendor/presets',
  });
}
