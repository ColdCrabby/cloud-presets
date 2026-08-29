/**
 * Runtime configuration for the public app. `apiBaseUrl` is left empty so the
 * generated client talks to the same origin; the dev-server proxy (see
 * proxy.conf.json) forwards `/v1` to the local stub API during development.
 */
export const environment = {
  production: false,
  apiBaseUrl: '',
};
