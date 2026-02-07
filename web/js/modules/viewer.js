/**
 * ScreenViewer — Remote screen viewing and input injection over WebSocket.
 * @module modules/viewer
 */

import { EventEmitter }    from '../core/events.js';
import { WebSocketClient } from '../core/websocket.js';

export class ScreenViewer extends EventEmitter {
    #canvas;
    #ctx;
    #ws           = null;
    #agentId      = null;
    #active       = false;
    #handlers     = {};
    #options;
    #pendingFrame = null;
    #rendering    = false;

    /**
     * @param {string|HTMLCanvasElement} canvas — Selector or element.
     * @param {Object} [options]
     * @param {boolean} [options.enableInput=true]
     */
    constructor(canvas, options = {}) {
        super();
        this.#canvas  = typeof canvas === 'string' ? document.querySelector(canvas) : canvas;
        this.#ctx     = this.#canvas.getContext('2d');
        this.#options = { enableInput: true, ...options };
    }

    /** Whether a viewer session is active. */
    get connected() { return this.#active; }

    /** The ID of the currently connected agent (or null). */
    get agentId() { return this.#agentId; }

    /**
     * Open a viewer session to the given agent.
     * @param {string} agentId
     * @returns {Promise<WebSocketClient>}
     */
    connect(agentId) {
        if (this.#active) this.disconnect();

        this.#agentId = agentId;
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const token = sessionStorage.getItem('rmm_api_key') || '';
        const url = `${protocol}//${location.host}/ws/viewer?agent=${agentId}&token=${encodeURIComponent(token)}`;

        this.#ws = new WebSocketClient(url, { reconnect: false });

        this.#ws.on('open', () => {
            this.#active = true;
            this.#attachInput();
            this.emit('connected', agentId);
        });

        this.#ws.on('close', () => {
            this.#active = false;
            this.#pendingFrame = null;
            this.#rendering = false;
            this.#detachInput();
            this.emit('disconnected', agentId);
        });

        this.#ws.on('binary',            (buf) => this.#handleBinary(buf));
        this.#ws.on('display_switched',   (msg) => this.emit('display_switched', msg.payload));
        this.#ws.on('error',              (err) => this.emit('error', err));

        return this.#ws.connect();
    }

    /** Close the active viewer session. */
    disconnect() {
        this.#ws?.close();
        this.#ws      = null;
        this.#active  = false;
        this.#agentId = null;
        this.#detachInput();
    }

    /**
     * Request the agent switch to a different display.
     * @param {number} displayNumber — 1-based display index.
     * @returns {boolean}
     */
    switchDisplay(displayNumber) {
        if (!this.#active) return false;
        return this.#ws.send({
            type: 'switch_display',
            payload: { display: displayNumber },
        });
    }

    /* Binary frame handling */

    /** Binary message type prefixes (must match protocol.Bin* constants). */
    static #BIN_SCREEN = 0x01;

    /**
     * Route an incoming binary WebSocket frame by its type prefix.
     * @param {ArrayBuffer} buffer
     */
    #handleBinary(buffer) {
        const view = new Uint8Array(buffer);
        if (view[0] === ScreenViewer.#BIN_SCREEN) {
            // Skip the 1-byte type prefix; queue the raw JPEG for rendering
            this.#pendingFrame = buffer.slice(1);
            if (!this.#rendering) this.#drainFrameQueue();
        }
    }

    /**
     * Render the most recent frame, skipping any that arrived while decoding.
     * Uses createImageBitmap for off-main-thread JPEG decode.
     */
    async #drainFrameQueue() {
        this.#rendering = true;

        while (this.#pendingFrame) {
            const jpeg = this.#pendingFrame;
            this.#pendingFrame = null;

            const blob   = new Blob([jpeg], { type: 'image/jpeg' });
            const bitmap = await createImageBitmap(blob);
            const w = bitmap.width;
            const h = bitmap.height;

            if (this.#canvas.width !== w || this.#canvas.height !== h) {
                this.#canvas.width  = w;
                this.#canvas.height = h;
            }

            this.#ctx.drawImage(bitmap, 0, 0);
            bitmap.close();

            this.emit('frame', { width: w, height: h });
        }

        this.#rendering = false;
    }

    /* Input handling */

    #attachInput() {
        if (!this.#options.enableInput) return;

        // Throttle mousemove to ≤60fps to avoid flooding the WebSocket
        let moveQueued = false;
        const throttledMove = (e) => {
            if (moveQueued) return;
            moveQueued = true;
            requestAnimationFrame(() => {
                this.#sendMouse('move', e);
                moveQueued = false;
            });
        };

        this.#handlers = {
            mousemove: throttledMove,
            mousedown: (e) => this.#sendMouse('down', e),
            mouseup:   (e) => this.#sendMouse('up', e),
            keydown:   (e) => { e.preventDefault(); this.#sendKey('down', e); },
            keyup:     (e) => { e.preventDefault(); this.#sendKey('up', e); },
        };

        this.#canvas.addEventListener('mousemove', this.#handlers.mousemove);
        this.#canvas.addEventListener('mousedown', this.#handlers.mousedown);
        this.#canvas.addEventListener('mouseup',   this.#handlers.mouseup);
        document.addEventListener('keydown',       this.#handlers.keydown);
        document.addEventListener('keyup',         this.#handlers.keyup);
    }

    #detachInput() {
        if (!this.#handlers.mousemove) return;
        this.#canvas.removeEventListener('mousemove', this.#handlers.mousemove);
        this.#canvas.removeEventListener('mousedown', this.#handlers.mousedown);
        this.#canvas.removeEventListener('mouseup',   this.#handlers.mouseup);
        document.removeEventListener('keydown',       this.#handlers.keydown);
        document.removeEventListener('keyup',         this.#handlers.keyup);
        this.#handlers = {};
    }

    #sendMouse(action, event) {
        if (!this.#active) return;
        const rect   = this.#canvas.getBoundingClientRect();
        const scaleX = this.#canvas.width  / rect.width;
        const scaleY = this.#canvas.height / rect.height;

        this.#ws.send({
            type: 'input',
            payload: {
                kind:   'mouse',
                action,
                button: event.button,
                x: Math.round((event.clientX - rect.left) * scaleX),
                y: Math.round((event.clientY - rect.top)  * scaleY),
            },
        });
    }

    #sendKey(action, event) {
        if (!this.#active) return;
        this.#ws.send({
            type: 'input',
            payload: {
                kind:   'key',
                action,
                key:    event.key,
                code:   event.keyCode,
            },
        });
    }
}
