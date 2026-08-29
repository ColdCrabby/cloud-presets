/**
 * Development configuration for the vendor-admin app. `apiBaseUrl` is
 * same-origin so requests reach the Go API serving this app. With an empty
 * `stytchPublicToken` the app uses a stub session. Production builds replace
 * this file with environment.prod.ts (see angular.json fileReplacements),
 * generated from STYTCH_PUBLIC_TOKEN by scripts/inject-stytch.mjs.
 */
export const environment = {
  production: false,
  apiBaseUrl: '',
  stytchPublicToken: '',
};
