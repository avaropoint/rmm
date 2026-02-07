/**
 * Modal management.
 * @module components/modal
 */

/**
 * Show a modal by adding the `active` class.
 * @param {string|HTMLElement} modal — CSS selector or element reference.
 */
export function showModal(modal) {
    const el = typeof modal === 'string' ? document.querySelector(modal) : modal;
    el?.classList.add('active');
}

/**
 * Hide a modal by removing the `active` class.
 * @param {string|HTMLElement} modal — CSS selector or element reference.
 */
export function hideModal(modal) {
    const el = typeof modal === 'string' ? document.querySelector(modal) : modal;
    el?.classList.remove('active');
}
