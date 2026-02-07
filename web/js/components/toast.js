/**
 * Toast notification system.
 * @module components/toast
 */

let container = null;

/**
 * Lazily create the toast container.
 * @returns {HTMLElement}
 */
function ensureContainer() {
    if (!container) {
        container = document.createElement('div');
        container.className = 'toast-container';
        document.body.appendChild(container);
    }
    return container;
}

/**
 * Show a toast notification.
 * @param {string} message
 * @param {'info'|'success'|'warning'|'error'} [type='info']
 * @param {number} [duration=3000] — milliseconds before auto-dismiss.
 */
export function toast(message, type = 'info', duration = 3000) {
    const el = document.createElement('div');
    el.className = `toast toast-${type}`;
    el.textContent = message;

    ensureContainer().appendChild(el);

    // Trigger entrance animation on next frame
    requestAnimationFrame(() => el.classList.add('show'));

    setTimeout(() => {
        el.classList.remove('show');
        el.addEventListener('transitionend', () => el.remove(), { once: true });
    }, duration);
}
