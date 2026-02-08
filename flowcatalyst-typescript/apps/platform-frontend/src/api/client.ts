/**
 * HTTP client configuration for Hey API generated SDK.
 * This sets up the base URL and any default headers.
 */

export const API_BASE_URL = '/api';
export const BFF_BASE_URL = '/bff';

/**
 * Custom error class for API errors that includes status code
 */
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

/**
 * Event emitter for API errors (401/403)
 */
type ApiErrorListener = (status: number, message: string) => void;
const errorListeners: ApiErrorListener[] = [];

export function onApiError(listener: ApiErrorListener): () => void {
  errorListeners.push(listener);
  return () => {
    const index = errorListeners.indexOf(listener);
    if (index > -1) {
      errorListeners.splice(index, 1);
    }
  };
}

function emitApiError(status: number, message: string) {
  errorListeners.forEach(listener => listener(status, message));
}

/**
 * Fetch from the main API endpoints.
 */
export async function apiFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  return baseFetch<T>(`${API_BASE_URL}${path}`, options);
}

/**
 * Fetch from BFF (Backend For Frontend) endpoints.
 * BFF endpoints return IDs as strings to preserve precision for JavaScript.
 */
export async function bffFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  return baseFetch<T>(`${BFF_BASE_URL}${path}`, options);
}

async function baseFetch<T>(
  url: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(url, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    const message = error.error || error.message || 'Request failed';

    // Emit error event for 401/403
    if (response.status === 401 || response.status === 403) {
      emitApiError(response.status, message);
    }

    throw new ApiError(message, response.status, error.code);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}
