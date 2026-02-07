/**
 * EventEmitter — Lightweight pub/sub event system.
 * @module core/events
 */

export class EventEmitter {
    #events = new Map();

    /**
     * Subscribe to an event.
     * @param {string} event
     * @param {Function} callback
     * @returns {this}
     */
    on(event, callback) {
        if (!this.#events.has(event)) {
            this.#events.set(event, []);
        }
        this.#events.get(event).push(callback);
        return this;
    }

    /**
     * Unsubscribe from an event. Omit callback to remove all listeners.
     * @param {string} event
     * @param {Function} [callback]
     * @returns {this}
     */
    off(event, callback) {
        if (!callback) {
            this.#events.delete(event);
        } else {
            const listeners = this.#events.get(event);
            if (listeners) {
                this.#events.set(event, listeners.filter(cb => cb !== callback));
            }
        }
        return this;
    }

    /**
     * Emit an event, invoking all registered callbacks.
     * @param {string} event
     * @param {...*} args
     * @returns {this}
     */
    emit(event, ...args) {
        const listeners = this.#events.get(event);
        if (!listeners) return this;
        for (const callback of listeners) {
            try {
                callback(...args);
            } catch (err) {
                console.error(`[EventEmitter] Error in "${event}" handler:`, err);
            }
        }
        return this;
    }

    /**
     * Subscribe to an event once — automatically unsubscribes after first emit.
     * @param {string} event
     * @param {Function} callback
     * @returns {this}
     */
    once(event, callback) {
        const wrapper = (...args) => {
            this.off(event, wrapper);
            callback(...args);
        };
        return this.on(event, wrapper);
    }
}
