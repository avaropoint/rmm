/**
 * HTTP client — Thin fetch wrapper with JSON handling and auth.
 * @module core/http
 */

/** @type {string|null} */
let _authToken = null;

/** Set the API key for all subsequent requests. */
export function setAuthToken(token) { _authToken = token; }

/** Get the current auth token. */
export function getAuthToken() { return _authToken; }

/**
 * Perform an HTTP request with automatic JSON serialisation/parsing.
 * Injects Authorization header when an auth token is set.
 * @param {string} url
 * @param {RequestInit} [options]
 * @returns {Promise<*>}
 */
export async function request(url, options = {}) {
    const config = {
        method: 'GET',
        ...options,
        headers: { ...options.headers },
    };

    // Inject auth header.
    if (_authToken) {
        config.headers['Authorization'] = `Bearer ${_authToken}`;
    }

    // Only set Content-Type for methods with a body (avoids unnecessary CORS preflight)
    if (config.body) {
        config.headers['Content-Type'] = config.headers['Content-Type'] ?? 'application/json';
        if (typeof config.body === 'object') {
            config.body = JSON.stringify(config.body);
        }
    }

    const response = await fetch(url, config);
    const contentType = response.headers.get('content-type');

    const data = contentType?.includes('application/json')
        ? await response.json()
        : await response.text();

    if (!response.ok) {
        const err = new Error(data?.error ?? data?.message ?? `HTTP ${response.status}`);
        err.status = response.status;
        throw err;
    }

    return data;
}

export const get  = (url, opts) => request(url, { ...opts, method: 'GET' });
export const post = (url, body, opts) => request(url, { ...opts, method: 'POST', body });
export const put  = (url, body, opts) => request(url, { ...opts, method: 'PUT', body });
export const del  = (url, opts) => request(url, { ...opts, method: 'DELETE' });
