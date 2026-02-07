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
import { get, post, del, setAuthToken, getAuthToken } from './core/http.js';

/* Selectors */

const SEL = Object.freeze({
    agents:           '#agents',
    agentCount:       '#agent-count',
    viewerModal:      '#viewer-modal',
    viewerTitle:      '#viewer-title',
    canvas:           '#screen',
    displayWrap:      '#display-selector',
    displaySelect:    '#display-select',
    loginOverlay:     '#login-overlay',
    loginForm:        '#login-form',
    loginError:       '#login-error',
    apiKeyInput:      '#api-key-input',
    enrollmentPanel:  '#enrollment-panel',
    enrollmentTokens: '#enrollment-tokens',
    enrollCodeDisplay:'#enrollment-code-display',
    enrollCodeValue:  '#enrollment-code-value',
});

/* State */

const agents = new AgentManager();
let   viewer = null;

/* ─── Authentication ─── */

const AUTH_KEY = 'rmm_api_key';

function isAuthenticated() {
    return !!getAuthToken();
}

function showLogin() {
    const overlay = document.querySelector(SEL.loginOverlay);
    if (overlay) overlay.hidden = false;
}

function hideLogin() {
    const overlay = document.querySelector(SEL.loginOverlay);
    if (overlay) overlay.hidden = true;
}

async function handleLogin(event) {
    event.preventDefault();
    const input = document.querySelector(SEL.apiKeyInput);
    const error = document.querySelector(SEL.loginError);
    const key = input?.value?.trim();

    if (!key) return;

    try {
        // Verify key with server (unauthenticated endpoint).
        const resp = await fetch('/api/auth/verify', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ key }),
        });
        if (!resp.ok) throw new Error('Invalid API key');

        setAuthToken(key);
        sessionStorage.setItem(AUTH_KEY, key);
        hideLogin();
        if (error) error.hidden = true;
        agents.startPolling();
        toast('Authenticated', 'success');
    } catch {
        if (error) {
            error.textContent = 'Invalid API key';
            error.hidden = false;
        }
    }
}

function handleLogout() {
    setAuthToken(null);
    sessionStorage.removeItem(AUTH_KEY);
    agents.stopPolling();
    showLogin();
}

/* ─── Enrollment Management ─── */

function toggleEnrollment() {
    const panel = document.querySelector(SEL.enrollmentPanel);
    if (!panel) return;
    panel.hidden = !panel.hidden;
    if (!panel.hidden) refreshTokens();
}

async function refreshTokens() {
    try {
        const tokens = await get('/api/enrollment');
        renderTokens(tokens ?? []);
    } catch (err) {
        toast('Failed to load tokens: ' + err.message, 'error');
    }
}

function renderTokens(tokens) {
    const tbody = document.querySelector(SEL.enrollmentTokens + ' tbody');
    if (!tbody) return;

    if (tokens.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;opacity:0.6">No tokens</td></tr>';
        return;
    }

    tbody.innerHTML = tokens.map(t => `
        <tr>
            <td><code>${escapeHtml(t.id)}</code></td>
            <td>${escapeHtml(t.type)}</td>
            <td>${escapeHtml(t.label || '—')}</td>
            <td>${formatRelativeTime(new Date(t.created_at))}</td>
            <td>${t.expires_at ? formatRelativeTime(new Date(t.expires_at)) : '—'}</td>
            <td><button class="btn btn-sm" data-action="delete-token" data-token-id="${t.id}">Delete</button></td>
        </tr>
    `).join('');
}

async function createToken(type) {
    try {
        const label = type === 'unattended' ? 'Unattended deployment' : '';
        const result = await post('/api/enrollment', { type, label });

        // Show the code.
        const display = document.querySelector(SEL.enrollCodeDisplay);
        const value = document.querySelector(SEL.enrollCodeValue);
        if (display && value) {
            value.textContent = result.code;
            display.hidden = false;
        }

        toast(`${type} token created`, 'success');
        refreshTokens();
    } catch (err) {
        toast('Failed to create token: ' + err.message, 'error');
    }
}

async function deleteToken(id) {
    try {
        await del(`/api/enrollment?id=${id}`);
        toast('Token deleted', 'success');
        refreshTokens();
    } catch (err) {
        toast('Failed to delete token: ' + err.message, 'error');
    }
}

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
        case 'toggle-enrollment':
            toggleEnrollment();
            break;
        case 'create-token':
            createToken(btn.dataset.tokenType);
            break;
        case 'delete-token':
            deleteToken(btn.dataset.tokenId);
            break;
        case 'logout':
            handleLogout();
            break;
    }
}

/* Bootstrap */

function init() {
    // Check for stored session.
    const savedKey = sessionStorage.getItem(AUTH_KEY);
    if (savedKey) {
        setAuthToken(savedKey);
        hideLogin();
    } else {
        showLogin();
    }

    // Login form handler.
    const loginForm = document.querySelector(SEL.loginForm);
    if (loginForm) loginForm.addEventListener('submit', handleLogin);

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

    // Agent polling (only when authenticated).
    agents.on('agents:changed', renderAgents);
    if (isAuthenticated()) agents.startPolling();

    // Global event delegation (replaces inline onclick handlers)
    document.addEventListener('click', handleGlobalClick);

    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') disconnectViewer();
    });
}

document.addEventListener('DOMContentLoaded', init);
