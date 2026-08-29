/**
 * Runtime configuration for the vendor-admin app. `apiBaseUrl` is same-origin so
 * the generated client's requests are forwarded to the stub API by the
 * dev-server proxy (see proxy.conf.json). `stytchPublicToken` is a placeholder
 * for the real Stytch B2B integration wired in the auth issue.
 */
export const environment = {
  production: false,
  apiBaseUrl: '',
  stytchPublicToken: 'public-token-placeholder',
};
