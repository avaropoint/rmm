/**
 * AgentManager — Polls the server for connected agents and emits state changes.
 * @module modules/agents
 */

import { EventEmitter } from '../core/events.js';
import { get } from '../core/http.js';

export class AgentManager extends EventEmitter {
    #agents = new Map();
    #pollTimer = null;
    #apiBase;

    /**
     * @param {string} [apiBase=''] — Base URL prefix for the agent API.
     */
    constructor(apiBase = '') {
        super();
        this.#apiBase = apiBase;
    }

    /**
     * Fetch the current agent list from the server and reconcile local state.
     * @returns {Promise<Object[]>}
     */
    async fetchAgents() {
        try {
            const list = (await get(`${this.#apiBase}/api/agents`)) ?? [];
            this.#reconcile(list);
            return this.all;
        } catch {
            // Silently swallow polling errors — server may be briefly unreachable
            return this.all;
        }
    }

    /**
     * Begin polling at the given interval.
     * @param {number} [interval=5000] — milliseconds between polls.
     */
    startPolling(interval = 5000) {
        this.stopPolling();
        this.fetchAgents();
        this.#pollTimer = setInterval(() => this.fetchAgents(), interval);
    }

    /** Stop the polling timer. */
    stopPolling() {
        if (this.#pollTimer) {
            clearInterval(this.#pollTimer);
            this.#pollTimer = null;
        }
    }

    /**
     * Retrieve a single agent by ID.
     * @param {string} id
     * @returns {Object|undefined}
     */
    get(id) {
        return this.#agents.get(id);
    }

    /** @returns {Object[]} — Snapshot of all known agents. */
    get all() {
        return [...this.#agents.values()];
    }

    /**
     * Diff incoming list against known state, emitting granular events.
     * Events: `agent:added`, `agent:updated`, `agent:removed`, `agents:changed`.
     */
    #reconcile(list) {
        const incoming = new Set(list.map(a => a.id));

        // Detect removed agents
        for (const id of this.#agents.keys()) {
            if (!incoming.has(id)) {
                this.#agents.delete(id);
                this.emit('agent:removed', id);
            }
        }

        // Upsert
        for (const agent of list) {
            const existing = this.#agents.get(agent.id);
            if (existing) {
                Object.assign(existing, agent);
                this.emit('agent:updated', existing);
            } else {
                this.#agents.set(agent.id, agent);
                this.emit('agent:added', agent);
            }
        }

        this.emit('agents:changed', this.all);
    }
}
