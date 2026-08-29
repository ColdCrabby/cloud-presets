/**
 * Runtime configuration for the vendor-admin app. `apiBaseUrl` is same-origin,
 * so requests reach the Go API that serves this app. Set `stytchPublicToken` to
 * a real Stytch B2B public token (`public-token-...`) to enable sign-in; left
 * empty, the app uses a stub session.
 */
export const environment = {
  production: false,
  apiBaseUrl: '',
  stytchPublicToken: 'public-token-placeholder',
};
