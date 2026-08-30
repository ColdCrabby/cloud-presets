/**
 * Runtime configuration for the public app. `apiBaseUrl` is left empty so the
 * generated client talks to the same origin; the dev-server proxy (see
 * proxy.conf.json) forwards `/v1` to the local stub API during development.
 *
 * `vendorUrl` is where the "Vendor login" link points. It is same-origin
 * (`/vendor/`) because both apps are served from one origin in dev and prod;
 * override it only if the vendor app is ever hosted elsewhere.
 */
export const environment = {
  production: false,
  apiBaseUrl: '',
  vendorUrl: '/vendor/',
};
