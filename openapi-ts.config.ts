import { defineConfig } from '@hey-api/openapi-ts';

export default defineConfig({
  input: './openapi/openapi.json',
  output: {
    path: './packages/api-client/src/generated',
    postProcess: ['prettier'],
  },
  plugins: ['@hey-api/client-fetch'],
});
