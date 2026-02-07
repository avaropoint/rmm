/**
 * ScreenViewer — Remote screen viewing and input injection over WebSocket.
 * @module modules/viewer
 */

import { EventEmitter }    from '../core/events.js';
import { WebSocketClient } from '../core/websocket.js';

export class ScreenViewer extends EventEmitter {
    #canvas;
    #ctx;
    #ws      = null;
    #agentId = null;
    #active  = false;
    #handlers = {};
    #options;

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
        const url = `${protocol}//${location.host}/ws/viewer?agent=${agentId}`;

        this.#ws = new WebSocketClient(url, { reconnect: false });

        this.#ws.on('open', () => {
            this.#active = true;
            this.#attachInput();
            this.emit('connected', agentId);
        });

        this.#ws.on('close', () => {
            this.#active = false;
            this.#detachInput();
            this.emit('disconnected', agentId);
        });

        this.#ws.on('screen',           (msg) => this.#renderFrame(msg.payload));
        this.#ws.on('display_switched',  (msg) => this.emit('display_switched', msg.payload));
        this.#ws.on('error',             (err) => this.emit('error', err));

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

    /* Frame rendering */

    #renderFrame(payload) {
        if (!payload?.data) return;

        const img = new Image();
        img.onload = () => {
            this.#canvas.width  = img.width;
            this.#canvas.height = img.height;
            this.#ctx.drawImage(img, 0, 0);
            URL.revokeObjectURL(img.src);
            this.emit('frame', { width: img.width, height: img.height });
        };

        // Decode base64 via fetch — faster than manual atob+charCode loop
        img.src = `data:image/jpeg;base64,${payload.data}`;
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
