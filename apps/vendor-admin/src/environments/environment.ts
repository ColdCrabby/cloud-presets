/**
 * Configuration for the vendor-admin app. `apiBaseUrl` is same-origin so
 * requests reach the Go API serving this app. `stytchPublicToken` is read at
 * runtime from `window.__APP_CONFIG__`, which the Go server injects into
 * index.html from the STYTCH_PUBLIC_TOKEN env var; empty means stub sign-in.
 */
declare global {
  interface Window {
    __APP_CONFIG__?: { stytchPublicToken?: string };
  }
}

export const environment = {
  production: false,
  apiBaseUrl: '',
  publicUrl: '/',
  stytchPublicToken: globalThis.window?.__APP_CONFIG__?.stytchPublicToken ?? '',
};
