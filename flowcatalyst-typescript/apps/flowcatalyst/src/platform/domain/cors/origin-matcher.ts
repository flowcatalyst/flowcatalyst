/**
 * CORS Origin Matcher
 *
 * Matches request origins against stored patterns that may include wildcards.
 * Supports patterns like:
 *   - https://example.com          (exact match)
 *   - https://*.example.com        (any subdomain)
 *   - https://qa-*.example.com     (prefix wildcard)
 *   - http://localhost:3000         (exact with port)
 */

/**
 * Check if a request origin matches any of the allowed origin patterns.
 *
 * @param origin - The request origin (e.g., "https://app.example.com")
 * @param allowedPatterns - Set of allowed origin patterns (may include wildcards)
 * @returns true if the origin matches any pattern
 */
export function isOriginAllowed(origin: string, allowedPatterns: ReadonlySet<string>): boolean {
  for (const pattern of allowedPatterns) {
    if (matchesOriginPattern(pattern, origin)) {
      return true;
    }
  }
  return false;
}

/**
 * Check if a request origin matches a single origin pattern.
 *
 * Wildcards (`*`) in the hostname are expanded to match `[a-zA-Z0-9-]+`.
 * Scheme and port must match exactly.
 *
 * @param pattern - Allowed origin pattern (e.g., "https://*.example.com")
 * @param origin - Request origin to check (e.g., "https://app.example.com")
 * @returns true if the origin matches the pattern
 */
export function matchesOriginPattern(pattern: string, origin: string): boolean {
  // Fast path: exact match
  if (pattern === origin) {
    return true;
  }

  // No wildcard — exact match only
  if (!pattern.includes('*')) {
    return false;
  }

  // Parse both into scheme + host + port
  try {
    const patternParts = parseOrigin(pattern);
    const originParts = parseOrigin(origin);

    if (!patternParts || !originParts) {
      return false;
    }

    // Scheme must match exactly
    if (patternParts.scheme !== originParts.scheme) {
      return false;
    }

    // Port must match exactly
    if (patternParts.port !== originParts.port) {
      return false;
    }

    // Convert wildcard host to regex: * -> [a-zA-Z0-9-]+
    const hostRegex = patternParts.host
      .replace(/[.]/g, '\\.')
      .replace(/\*/g, '[a-zA-Z0-9-]+');

    return new RegExp(`^${hostRegex}$`).test(originParts.host);
  } catch {
    return false;
  }
}

interface OriginParts {
  scheme: string;
  host: string;
  port: string;
}

function parseOrigin(origin: string): OriginParts | null {
  const match = origin.match(/^(https?):\/\/([^:/]+)(?::(\d+))?$/);
  if (!match) return null;

  return {
    scheme: match[1]!,
    host: match[2]!,
    port: match[3] ?? '',
  };
}
