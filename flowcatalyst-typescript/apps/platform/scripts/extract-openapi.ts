/**
 * OpenAPI Spec Extraction Script
 *
 * Starts the Fastify server in a minimal mode, extracts the generated
 * OpenAPI specification from @fastify/swagger, and writes it to disk.
 *
 * Usage: tsx scripts/extract-openapi.ts
 */

import { writeFile, mkdir } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const outputDir = resolve(__dirname, '../openapi');

async function main() {
  // Set env defaults for extraction (no real DB needed if we just need schema)
  process.env['NODE_ENV'] = 'production';
  process.env['LOG_LEVEL'] = 'error';

  // Dynamic import to pick up env overrides
  const { startPlatform } = await import('../src/index.js');

  console.log('Starting platform to extract OpenAPI spec...');

  let fastify;
  try {
    fastify = await startPlatform({
      port: 0, // Random available port
      host: '127.0.0.1',
      logLevel: 'error',
    });

    // Wait for swagger to be ready
    await fastify.ready();

    // Get the OpenAPI spec
    const spec = fastify.swagger();

    if (!spec || !spec.openapi) {
      console.error('Failed to extract OpenAPI spec - swagger() returned empty');
      process.exit(1);
    }

    // Ensure output directory exists
    await mkdir(outputDir, { recursive: true });

    // Write JSON
    const jsonPath = resolve(outputDir, 'openapi.json');
    await writeFile(jsonPath, JSON.stringify(spec, null, 2), 'utf-8');
    console.log(`Written: ${jsonPath}`);

    // Write YAML (simple JSON-to-YAML conversion)
    const yamlPath = resolve(outputDir, 'openapi.yaml');
    const yaml = jsonToYaml(spec);
    await writeFile(yamlPath, yaml, 'utf-8');
    console.log(`Written: ${yamlPath}`);

    const routeCount = Object.keys(spec.paths ?? {}).length;
    console.log(`\nOpenAPI spec extracted successfully (${routeCount} paths)`);
  } catch (err) {
    console.error('Failed to extract OpenAPI spec:', err);
    process.exit(1);
  } finally {
    if (fastify) {
      await fastify.close();
    }
  }
}

/**
 * Simple JSON to YAML converter (no dependency needed).
 */
function jsonToYaml(obj: unknown, indent = 0): string {
  const prefix = '  '.repeat(indent);

  if (obj === null || obj === undefined) {
    return 'null';
  }

  if (typeof obj === 'string') {
    // Quote strings that could be ambiguous
    if (
      obj === '' ||
      obj.includes(':') ||
      obj.includes('#') ||
      obj.includes('\n') ||
      obj.startsWith('{') ||
      obj.startsWith('[') ||
      obj.startsWith('"') ||
      obj.startsWith("'") ||
      obj === 'true' ||
      obj === 'false' ||
      obj === 'null' ||
      /^\d/.test(obj)
    ) {
      return JSON.stringify(obj);
    }
    return obj;
  }

  if (typeof obj === 'number' || typeof obj === 'boolean') {
    return String(obj);
  }

  if (Array.isArray(obj)) {
    if (obj.length === 0) return '[]';
    return obj
      .map((item) => {
        const value = jsonToYaml(item, indent + 1);
        if (typeof item === 'object' && item !== null) {
          return `${prefix}- ${value.trimStart()}`;
        }
        return `${prefix}- ${value}`;
      })
      .join('\n');
  }

  if (typeof obj === 'object') {
    const entries = Object.entries(obj as Record<string, unknown>);
    if (entries.length === 0) return '{}';

    return entries
      .map(([key, value]) => {
        const yamlKey = /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(key) ? key : JSON.stringify(key);

        if (value === null || value === undefined) {
          return `${prefix}${yamlKey}: null`;
        }

        if (typeof value === 'object' && !Array.isArray(value)) {
          const inner = jsonToYaml(value, indent + 1);
          return `${prefix}${yamlKey}:\n${inner}`;
        }

        if (Array.isArray(value)) {
          if (value.length === 0) {
            return `${prefix}${yamlKey}: []`;
          }
          const inner = jsonToYaml(value, indent + 1);
          return `${prefix}${yamlKey}:\n${inner}`;
        }

        return `${prefix}${yamlKey}: ${jsonToYaml(value, indent)}`;
      })
      .join('\n');
  }

  return String(obj);
}

main();
