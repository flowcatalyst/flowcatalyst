import { defineConfig } from '@hey-api/openapi-ts';

// Use local file from platform build, or live endpoint for dev
const openApiInput = process.env.OPENAPI_LIVE === 'true'
  ? 'http://localhost:8080/q/openapi'
  : './openapi/openapi.yaml';

export default defineConfig({
  input: openApiInput,
  output: {
    path: 'src/generated',
  },
  plugins: [
    '@hey-api/typescript',
    '@hey-api/sdk',
    '@hey-api/client-fetch',
  ],
  postProcess: ['prettier'],
});
