/**
 * WebSocketClient — Managed WebSocket with auto-reconnect.
 * @module core/websocket
 */

import { EventEmitter } from './events.js';

export class WebSocketClient extends EventEmitter {
    #ws = null;
    #url;
    #options;
    #reconnectAttempts = 0;
    #intentionalClose = false;

    /**
     * @param {string} url — WebSocket endpoint.
     * @param {Object} [options]
     * @param {boolean} [options.reconnect=true]
     * @param {number}  [options.reconnectInterval=3000]
     * @param {number}  [options.maxReconnectAttempts=10]
     */
    constructor(url, options = {}) {
        super();
        this.#url = url;
        this.#options = {
            reconnect: true,
            reconnectInterval: 3000,
            maxReconnectAttempts: 10,
            ...options,
        };
    }

    /** Whether the socket is currently open. */
    get connected() {
        return this.#ws?.readyState === WebSocket.OPEN;
    }

    /**
     * Open (or reuse) the WebSocket connection.
     * @returns {Promise<this>}
     */
    connect() {
        if (this.connected) return Promise.resolve(this);

        return new Promise((resolve, reject) => {
            this.#ws = new WebSocket(this.#url);
            this.#ws.binaryType = 'arraybuffer';
            this.#intentionalClose = false;

            this.#ws.onopen = () => {
                this.#reconnectAttempts = 0;
                this.emit('open');
                resolve(this);
            };

            this.#ws.onclose = (event) => {
                this.emit('close', event);
                if (!this.#intentionalClose && this.#options.reconnect) {
                    this.#reconnect();
                }
            };

            this.#ws.onerror = (error) => {
                this.emit('error', error);
                reject(error);
            };

            this.#ws.onmessage = ({ data: raw }) => {
                // Binary frames: emit as ArrayBuffer for high-performance paths
                if (raw instanceof ArrayBuffer) {
                    this.emit('binary', raw);
                    return;
                }

                // Text frames: parse JSON, emit by type
                try {
                    const data = JSON.parse(raw);
                    this.emit('message', data);
                    if (data.type) this.emit(data.type, data);
                } catch {
                    this.emit('message', raw);
                }
            };
        });
    }

    /**
     * Send data over the socket. Objects are JSON-serialised automatically.
     * @param {string|Object} data
     * @returns {boolean} — true if sent, false if not connected.
     */
    send(data) {
        if (!this.connected) return false;
        this.#ws.send(typeof data === 'string' ? data : JSON.stringify(data));
        return true;
    }

    /** Gracefully close the socket (suppresses auto-reconnect). */
    close() {
        this.#intentionalClose = true;
        this.#ws?.close();
    }

    #reconnect() {
        if (this.#reconnectAttempts >= this.#options.maxReconnectAttempts) {
            this.emit('reconnect_failed');
            return;
        }
        this.#reconnectAttempts++;
        this.emit('reconnecting', this.#reconnectAttempts);
        setTimeout(() => this.connect().catch(() => {}), this.#options.reconnectInterval);
    }
}
