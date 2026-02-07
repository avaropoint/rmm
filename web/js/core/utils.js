/**
 * Shared utility functions.
 * @module core/utils
 */

const OS_LABELS = Object.freeze({
    darwin:  'macOS',
    linux:   'Linux',
    windows: 'Windows',
    freebsd: 'FreeBSD',
    android: 'Android',
    ios:     'iOS',
});

/**
 * Escape HTML entities to prevent XSS.
 * @param {string} text
 * @returns {string}
 */
export function escapeHtml(text) {
    if (!text) return '';
    const el = document.createElement('span');
    el.textContent = text;
    return el.innerHTML;
}

/**
 * Convert Go runtime.GOOS identifiers to friendly display names.
 * @param {string} os
 * @returns {string}
 */
export function formatOS(os) {
    return OS_LABELS[os?.toLowerCase()] ?? os ?? 'Unknown';
}

/**
 * Strip port and IPv6 brackets from an address string.
 * @param {string} ip
 * @returns {string}
 */
export function formatIP(ip) {
    if (!ip) return 'Unknown';
    // IPv6 with brackets: [::1]:port
    if (ip.startsWith('[')) {
        const end = ip.indexOf(']');
        return end > 0 ? ip.slice(1, end) : ip;
    }
    // IPv4: 1.2.3.4:port (only strip if exactly one colon)
    const last = ip.lastIndexOf(':');
    if (last > 0 && ip.indexOf(':') === last) {
        return ip.slice(0, last);
    }
    return ip;
}

/**
 * Human-friendly relative timestamp.
 * @param {Date} date
 * @returns {string}
 */
export function formatRelativeTime(date) {
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
    if (seconds < 5)     return 'Just now';
    if (seconds < 60)    return `${seconds}s ago`;
    if (seconds < 3600)  return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    return `${Math.floor(seconds / 86400)}d ago`;
}

/**
 * Format bytes to a human-readable string (e.g. 16.0 GB).
 * @param {number} bytes
 * @returns {string}
 */
export function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

/**
 * Format seconds to a human-readable uptime string.
 * @param {number} seconds
 * @returns {string}
 */
export function formatUptime(seconds) {
    if (!seconds || seconds <= 0) return 'Unknown';
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
}

/**
 * Format display info array to a readable string.
 * @param {Array} displays
 * @returns {string}
 */
export function formatDisplays(displays) {
    if (!displays?.length) return 'Unknown';
    return displays.map(d => `${d.width}×${d.height}`).join(', ');
}

/**
 * Generate a short unique identifier.
 * @returns {string}
 */
export function generateId() {
    return crypto.randomUUID().replaceAll('-', '').slice(0, 16);
}
