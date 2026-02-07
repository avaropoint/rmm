/**
 * HTTP client — Thin fetch wrapper with JSON handling.
 * @module core/http
 */

/**
 * Perform an HTTP request with automatic JSON serialisation/parsing.
 * @param {string} url
 * @param {RequestInit} [options]
 * @returns {Promise<*>}
 */
export async function request(url, options = {}) {
    const config = {
        method: 'GET',
        ...options,
    };

    // Only set Content-Type for methods with a body (avoids unnecessary CORS preflight)
    if (config.body) {
        config.headers = { 'Content-Type': 'application/json', ...config.headers };
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
        throw new Error(data?.message ?? `HTTP ${response.status}`);
    }

    return data;
}

export const get  = (url, opts) => request(url, { ...opts, method: 'GET' });
export const post = (url, body, opts) => request(url, { ...opts, method: 'POST', body });
export const put  = (url, body, opts) => request(url, { ...opts, method: 'PUT', body });
export const del  = (url, opts) => request(url, { ...opts, method: 'DELETE' });
