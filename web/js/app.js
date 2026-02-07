/**
 * Dashboard — Application entry point.
 *
 * Composes framework modules into the management dashboard.
 * No globals are created; all interaction uses data-attribute event delegation.
 *
 * @module app
 */

import { AgentManager }               from './modules/agents.js';
import { ScreenViewer }                from './modules/viewer.js';
import { showModal, hideModal }        from './components/modal.js';
import { toast }                       from './components/toast.js';
import { Icons }                       from './components/icons.js';
import { escapeHtml, formatOS, formatIP,
         formatRelativeTime, formatBytes,
         formatUptime, formatDisplays }  from './core/utils.js';

/* Selectors */

const SEL = Object.freeze({
    agents:        '#agents',
    agentCount:    '#agent-count',
    viewerModal:   '#viewer-modal',
    viewerTitle:   '#viewer-title',
    canvas:        '#screen',
    displayWrap:   '#display-selector',
    displaySelect: '#display-select',
});

/* State */

const agents = new AgentManager();
let   viewer = null;

/* Agent card rendering */

function renderAgents(list) {
    const container = document.querySelector(SEL.agents);
    if (!container) return;

    // Update header count
    const countEl = document.querySelector(SEL.agentCount);
    if (countEl) countEl.textContent = list.length;

    if (list.length === 0) {
        container.innerHTML = `
            <div class="empty-state">
                <div class="empty-state-icon">${Icons.devices}</div>
                <div class="empty-state-title">No agents connected</div>
                <div class="empty-state-description">
                    Deploy an agent to a device to begin remote management
                </div>
            </div>`;
        return;
    }

    container.innerHTML = '';
    for (const agent of list) {
        container.appendChild(buildAgentCard(agent));
    }
}

function buildAgentCard(agent) {
    const card = document.createElement('div');
    card.className = 'card';
    card.dataset.agentId = agent.id;

    const lastSeen = agent.last_seen
        ? formatRelativeTime(new Date(agent.last_seen))
        : 'Unknown';

    card.innerHTML = `
        <div class="card-header">
            <div class="agent-info">
                <div class="agent-name">${escapeHtml(agent.name ?? agent.hostname)}</div>
                <div class="agent-id">${agent.id}</div>
            </div>
            <div class="status-indicator ${agent.status === 'online' ? '' : 'offline'}">
                <span class="status-dot ${agent.status === 'online' ? '' : 'offline'}"></span>
                <span class="status-label">${agent.status ?? 'online'}</span>
            </div>
        </div>
        <div class="card-body">
            <div class="agent-detail">
                <span class="agent-detail-label">System</span>
                <span class="agent-detail-value">${escapeHtml(agent.os_version || formatOS(agent.os))} / ${escapeHtml(agent.arch ?? 'Unknown')}</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">User</span>
                <span class="agent-detail-value">${escapeHtml(agent.username || 'Unknown')}</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">Displays</span>
                <span class="agent-detail-value">${formatDisplays(agent.displays)} (${agent.display_count ?? 1})</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">CPU</span>
                <span class="agent-detail-value">${agent.cpu_count ?? 0} cores</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">Memory</span>
                <span class="agent-detail-value">${formatBytes(agent.memory_free)} / ${formatBytes(agent.memory_total)}</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">Disk</span>
                <span class="agent-detail-value">${formatBytes(agent.disk_free)} / ${formatBytes(agent.disk_total)}</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">IP</span>
                <span class="agent-detail-value">${escapeHtml(formatIP(agent.ip))}</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">Uptime</span>
                <span class="agent-detail-value">${formatUptime(agent.uptime_seconds)}</span>
            </div>
            <div class="agent-detail">
                <span class="agent-detail-label">Seen</span>
                <span class="agent-detail-value">${lastSeen}</span>
            </div>
        </div>
        <div class="card-footer">
            <button class="btn btn-primary btn-block"
                    data-action="connect"
                    data-agent-id="${agent.id}">
                <span class="btn-icon">${Icons.play}</span>
                Connect
            </button>
        </div>`;

    return card;
}

/* Display switching */

function setupDisplaySelector(agent) {
    const wrap   = document.querySelector(SEL.displayWrap);
    const select = document.querySelector(SEL.displaySelect);
    if (!wrap || !select) return;

    const count = agent.display_count ?? 1;

    if (count <= 1) {
        wrap.style.display = 'none';
        return;
    }

    select.innerHTML = '';
    for (let i = 1; i <= count; i++) {
        const opt = document.createElement('option');
        opt.value = i;
        opt.textContent = `Display ${i}`;
        select.appendChild(opt);
    }

    wrap.style.display = 'flex';
    select.onchange = () => {
        viewer?.switchDisplay(parseInt(select.value, 10));
    };
}

/* Connection lifecycle */

function connectToAgent(agentId) {
    if (!viewer) return;

    const agent = agents.get(agentId);
    if (agent) setupDisplaySelector(agent);

    viewer.connect(agentId).catch(() => {
        toast('Failed to connect to agent', 'error');
    });
}

function disconnectViewer() {
    viewer?.disconnect();
}

/* Event delegation */

function handleGlobalClick(event) {
    const btn = event.target.closest('[data-action]');
    if (!btn) return;

    switch (btn.dataset.action) {
        case 'connect':
            connectToAgent(btn.dataset.agentId);
            break;
        case 'disconnect':
            disconnectViewer();
            break;
    }
}

/* Bootstrap */

function init() {
    // Screen viewer
    const canvas = document.querySelector(SEL.canvas);
    if (canvas) {
        viewer = new ScreenViewer(canvas);
        viewer.on('connected',    () => showModal(SEL.viewerModal));
        viewer.on('disconnected', () => hideModal(SEL.viewerModal));
        viewer.on('display_switched', (payload) => {
            const select = document.querySelector(SEL.displaySelect);
            if (select && payload?.display) select.value = payload.display;
        });
    }

    // Agent polling
    agents.on('agents:changed', renderAgents);
    agents.startPolling();

    // Global event delegation (replaces inline onclick handlers)
    document.addEventListener('click', handleGlobalClick);

    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') disconnectViewer();
    });
}

document.addEventListener('DOMContentLoaded', init);
