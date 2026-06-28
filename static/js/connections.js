window.Connections = (function() {
    let container = null;
    let wsConn = null;
    let reconnectTimer = null;
    let reconnectDelay = 1000;
    const MAX_RECONNECT_DELAY = 30000;
    const MIN_RENDER_INTERVAL = 200;
    const MAX_CLOSED_COUNT = 100;

    let activeConnMap = new Map();
    let closedConnMap = new Map();
    let prevSnapshot = new Map();

    let sortBy = 'host';
    let sortAsc = false;
    let filterText = '';
    let currentTab = 'active';
    
    let rafPending = false;
    let isPaused = false;
    let lastRenderTime = 0;
    let pendingRenderTimer = null;
    let langEventListener = null;

    function t(key) {
        return (window.i18n && window.i18n.t) ? window.i18n.t(key) : key;
    }

    function formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/[&<>"']/g, m => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        })[m]);
    }

    function getHost(meta) {
        return meta?.host || meta?.destinationIP || meta?.sourceIP || '?';
    }

    function trimClosedConnections() {
        if (closedConnMap.size > MAX_CLOSED_COUNT) {
            const keys = Array.from(closedConnMap.keys());
            const toDelete = keys.slice(0, closedConnMap.size - MAX_CLOSED_COUNT);
            for (const k of toDelete) closedConnMap.delete(k);
        }
    }

    function scheduleRender(force = false) {
        if (isPaused && !force) return;
        const now = performance.now();
        const elapsed = now - lastRenderTime;
        if (elapsed >= MIN_RENDER_INTERVAL || force) {
            if (rafPending) cancelAnimationFrame(rafPending);
            rafPending = requestAnimationFrame(() => {
                rafPending = false;
                lastRenderTime = performance.now();
                renderCards();
            });
        } else if (!pendingRenderTimer) {
            pendingRenderTimer = setTimeout(() => {
                pendingRenderTimer = null;
                scheduleRender(force);
            }, MIN_RENDER_INTERVAL - elapsed);
        }
    }

    function renderCards() {
        const grid = document.getElementById('conn-grid');
        const pauseBtn = document.getElementById('toggle-pause');
        const tabActive = document.getElementById('tab-active');
        const tabClosed = document.getElementById('tab-closed');
        const clearAllBtn = document.getElementById('clear-all-closed');
        if (!grid) return;

        if (pauseBtn) {
            pauseBtn.textContent = isPaused ? t('connections.resume') : t('connections.pause');
            pauseBtn.classList.toggle('paused', isPaused);
        }

        if (tabActive) tabActive.textContent = t('connections.active') + ' (' + activeConnMap.size + ')';
        if (tabClosed) tabClosed.textContent = t('connections.closed') + ' (' + closedConnMap.size + ')';

        if (clearAllBtn) {
            clearAllBtn.style.display = currentTab === 'closed' ? 'inline-flex' : 'none';
        }

        let dataMap = currentTab === 'active' ? activeConnMap : closedConnMap;
        let rows = Array.from(dataMap.values());

        if (filterText) {
            const lower = filterText.toLowerCase();
            rows = rows.filter(c =>
                getHost(c.meta).toLowerCase().includes(lower) ||
                (c.rule || '').toLowerCase().includes(lower) ||
                (c.chain || []).some(p => p.toLowerCase().includes(lower))
            );
        }

        rows.sort((a, b) => {
            let aVal, bVal;
            switch (sortBy) {
                case 'host':   aVal = getHost(a.meta); bVal = getHost(b.meta); break;
                case 'port':   aVal = a.meta?.destinationPort || ''; bVal = b.meta?.destinationPort || ''; break;
                case 'rule':   aVal = a.rule || ''; bVal = b.rule || ''; break;
                case 'chain':  aVal = (a.chain || []).join(' → '); bVal = (b.chain || []).join(' → '); break;
                case 'uploadSpeed':   aVal = a.speedUp || 0; bVal = b.speedUp || 0; break;
                case 'downloadSpeed': aVal = a.speedDown || 0; bVal = b.speedDown || 0; break;
                default: aVal = a.downloadSpeed || 0; bVal = b.downloadSpeed || 0;
            }
            if (typeof aVal === 'string') { aVal = aVal.toLowerCase(); bVal = bVal.toLowerCase(); }
            if (aVal < bVal) return sortAsc ? -1 : 1;
            if (aVal > bVal) return sortAsc ? 1 : -1;
            return 0;
        });

        grid.innerHTML = '';
        if (rows.length === 0) {
            grid.innerHTML = `<div class="empty-state">${t('connections.empty')}</div>`;
            return;
        }

        const fragment = document.createDocumentFragment();
        for (const conn of rows) {
            const host = getHost(conn.meta);
            const port = conn.meta?.destinationPort || '';
            const rule = conn.rule || 'DIRECT';
            const chain = (conn.chain || []).join(' → ') || '-';
            const isClosed = currentTab === 'closed';

            const card = document.createElement('div');
            card.className = 'conn-card' + (isClosed ? ' closed-card' : '');
            card.dataset.id = conn.id;

            let actionButton = '';
            if (isClosed) {
                actionButton = `<button class="clear-conn-btn" data-id="${escapeHtml(conn.id)}" title="${t('connections.clear')}">✕</button>`;
            } else {
                actionButton = `<button class="close-conn-btn" data-id="${escapeHtml(conn.id)}">${t('connections.close')}</button>`;
            }

            card.innerHTML = `
                <div class="conn-card-header">
                    <span class="conn-host" title="${escapeHtml(host)}">${escapeHtml(host)}</span>
                    <span class="conn-port">:${escapeHtml(port)}</span>
                    ${actionButton}
                </div>
                <div class="conn-card-body">
                    <div class="conn-detail"><span class="label">${t('connections.rule')}</span><span>${escapeHtml(rule)}</span></div>
                    <div class="conn-detail"><span class="label">${t('connections.chain')}</span><span title="${escapeHtml(chain)}">${escapeHtml(chain)}</span></div>
                </div>
            `;

            if (isClosed) {
                const clearBtn = card.querySelector('.clear-conn-btn');
                clearBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    clearClosedConnection(e.currentTarget.dataset.id);
                });
            } else {
                const closeBtn = card.querySelector('.close-conn-btn');
                closeBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    closeConnection(e.currentTarget.dataset.id);
                });
            }

            fragment.appendChild(card);
        }
        grid.appendChild(fragment);
    }

    function processConnections(data) {
        const now = performance.now();
        const newActiveMap = new Map();
        const connectionsList = data.connections || [];

        for (const conn of connectionsList) {
            const prev = prevSnapshot.get(conn.id);
            let speedUp = 0, speedDown = 0;
            if (prev) {
                const timeDiff = (now - prev.timestamp) / 1000;
                if (timeDiff > 0.05) {
                    speedUp = Math.max(0, (conn.upload - prev.upload) / timeDiff);
                    speedDown = Math.max(0, (conn.download - prev.download) / timeDiff);
                }
            }
            newActiveMap.set(conn.id, {
                id: conn.id,
                meta: conn.metadata,
                upload: conn.upload,
                download: conn.download,
                speedUp,
                speedDown,
                rule: conn.rule,
                chain: conn.chains,
                timestamp: now
            });
        }

        const oldActiveIds = new Set(activeConnMap.keys());
        const newActiveIds = new Set(newActiveMap.keys());
        for (const id of oldActiveIds) {
            if (!newActiveIds.has(id)) {
                const closedConn = activeConnMap.get(id);
                if (closedConn) {
                    const { speedUp, speedDown, ...rest } = closedConn;
                    closedConnMap.set(id, rest);
                }
            }
        }

        activeConnMap = newActiveMap;

        prevSnapshot.clear();
        for (const conn of connectionsList) {
            prevSnapshot.set(conn.id, { upload: conn.upload, download: conn.download, timestamp: now });
        }

        trimClosedConnections();

        const tabActive = document.getElementById('tab-active');
        const tabClosed = document.getElementById('tab-closed');
        if (tabActive) tabActive.textContent = t('connections.active') + ' (' + activeConnMap.size + ')';
        if (tabClosed) tabClosed.textContent = t('connections.closed') + ' (' + closedConnMap.size + ')';

        scheduleRender();
    }

    async function closeConnection(id) {
        const conn = activeConnMap.get(id);
        if (conn) {
            activeConnMap.delete(id);
            const { speedUp, speedDown, ...rest } = conn;
            closedConnMap.set(id, rest);
            trimClosedConnections();
            scheduleRender(true);
        }

        try {
            await window.API.apiFetch(`/connections/${encodeURIComponent(id)}`, { method: 'DELETE' });
        } catch (err) {
            console.error('断开连接失败:', err);
            showToast(t('connections.close_failed') + ': ' + err.message, 'error');
        }
    }

    async function closeAllConnections() {
        if (!confirm(t('connections.confirm_close_all'))) return;
        for (const [id, conn] of activeConnMap) {
            const { speedUp, speedDown, ...rest } = conn;
            closedConnMap.set(id, rest);
        }
        activeConnMap.clear();
        prevSnapshot.clear();
        trimClosedConnections();
        scheduleRender(true);

        try {
            await window.API.apiFetch('/connections', { method: 'DELETE' });
            showToast(t('connections.close_all_success'), 'success');
        } catch (err) {
            console.error('断开所有连接失败:', err);
            showToast(t('connections.close_all_failed'), 'error');
        }
    }

    function clearClosedConnection(id) {
        if (closedConnMap.has(id)) {
            closedConnMap.delete(id);
            scheduleRender(true);
        }
    }

    function clearAllClosed() {
        if (closedConnMap.size === 0) return;
        if (!confirm(t('connections.confirm_clear_closed') || '确定清空所有已关闭连接吗？')) return;
        closedConnMap.clear();
        scheduleRender(true);
        showToast(t('connections.cleared_closed') || '已清空所有已关闭连接', 'success');
    }

    function showToast(msg, type = 'info') {
        let toast = document.getElementById('conn-toast');
        if (!toast) {
            toast = document.createElement('div');
            toast.id = 'conn-toast';
            toast.style.cssText = 'position:fixed;top:20px;right:20px;z-index:9999;padding:12px 20px;border-radius:8px;font-size:14px;color:#fff;transition:opacity 0.3s;pointer-events:none;';
            document.body.appendChild(toast);
        }
        toast.textContent = msg;
        toast.style.background = type === 'error' ? '#ef4444' : type === 'success' ? '#22c55e' : '#3b82f6';
        toast.style.opacity = '1';
        clearTimeout(toast._timer);
        toast._timer = setTimeout(() => { toast.style.opacity = '0'; }, 2500);
    }

    function startWebSocket() {
        if (wsConn) { wsConn.close(); wsConn = null; }
        wsConn = window.API.wsConnect('/connections', (e) => {
            try {
                processConnections(JSON.parse(e.data));
                reconnectDelay = 1000;
            } catch (err) { console.warn('连接数据解析错误', err); }
        });
        const origClose = wsConn.onclose;
        wsConn.onclose = (e) => {
            if (origClose) origClose(e);
            if (reconnectTimer) clearTimeout(reconnectTimer);
            reconnectTimer = setTimeout(startWebSocket, reconnectDelay);
            reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY);
        };
    }

    function setSort(column) {
        if (sortBy === column) sortAsc = !sortAsc;
        else { sortBy = column; sortAsc = false; }
        document.getElementById('sort-select').value = column;
        updateSortButton();
        scheduleRender(true);
    }

    function updateSortButton() {
        const btn = document.getElementById('sort-dir-btn');
        if (!btn) return;
        const svg = btn.querySelector('svg');
        if (svg) {
            svg.style.transform = sortAsc ? 'rotate(180deg)' : 'rotate(0deg)';
        }
    }

    function refreshUI() {
        renderCards();
        const filterInput = document.getElementById('conn-filter');
        if (filterInput) filterInput.placeholder = t('connections.search_placeholder');
        const pauseBtn = document.getElementById('toggle-pause');
        if (pauseBtn) pauseBtn.textContent = isPaused ? t('connections.resume') : t('connections.pause');
        const closeAllBtn = document.getElementById('close-all-connections');
        if (closeAllBtn) closeAllBtn.textContent = t('connections.close_all');
        const clearAllBtn = document.getElementById('clear-all-closed');
        if (clearAllBtn) clearAllBtn.textContent = t('connections.clear_all_closed') || '清空全部';
        const sortBtn = document.getElementById('sort-dir-btn');
        if (sortBtn) {
            const textSpan = sortBtn.querySelector('.sort-label');
            if (textSpan) textSpan.textContent = t('connections.sort');
        }
        const tabActive = document.getElementById('tab-active');
        const tabClosed = document.getElementById('tab-closed');
        if (tabActive) tabActive.textContent = t('connections.active') + ' (' + activeConnMap.size + ')';
        if (tabClosed) tabClosed.textContent = t('connections.closed') + ' (' + closedConnMap.size + ')';
        const options = document.querySelectorAll('#sort-select option');
        if (options.length >= 6) {
            options[0].textContent = t('connections.host');
            options[1].textContent = t('connections.port');
            options[2].textContent = t('connections.rule');
            options[3].textContent = t('connections.chain');
            options[4].textContent = t('connections.upload_speed');
            options[5].textContent = t('connections.download_speed');
        }
    }

    function onLanguageChange() {
        refreshUI();
    }

    function initLanguageListener() {
        if (langEventListener) window.removeEventListener('languageChanged', langEventListener);
        langEventListener = onLanguageChange;
        window.addEventListener('languageChanged', langEventListener);
    }

    function render() {
        if (!container) return;
        container.innerHTML = `
            <style>
                .conn-toolbar {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    flex-wrap: wrap;
                    gap: 12px;
                    margin-bottom: 16px;
                }
                .conn-tabs {
                    display: flex;
                    gap: 4px;
                    background: var(--bg-secondary, #f1f5f9);
                    padding: 4px;
                    border-radius: 8px;
                }
                .conn-tab {
                    padding: 6px 16px;
                    border: none;
                    border-radius: 6px;
                    background: transparent;
                    color: var(--text-secondary, #64748b);
                    cursor: pointer;
                    font-size: 14px;
                    font-weight: 500;
                    transition: all 0.2s;
                }
                .conn-tab.active {
                    background: var(--primary-color, #3b82f6);
                    color: #fff;
                    font-weight: 600;
                    box-shadow: 0 2px 4px rgba(59,130,246,0.3);
                }
                .conn-tab:hover:not(.active) {
                    background: rgba(255,255,255,0.6);
                }
                .conn-actions {
                    display: flex;
                    gap: 8px;
                    align-items: center;
                    flex-wrap: wrap;
                }
                .btn {
                    padding: 6px 14px;
                    border: none;
                    border-radius: 6px;
                    cursor: pointer;
                    font-size: 13px;
                    transition: all 0.2s;
                    display: inline-flex;
                    align-items: center;
                    gap: 4px;
                }
                .btn-pause {
                    background: #d1d5db;
                    color: var(--text-primary, #1e293b);
                    border: 1px solid var(--border-color, #cbd5e1);
                    box-shadow: 0 1px 2px rgba(0,0,0,0.04);
                }
                .btn-pause.paused {
                    background: var(--warning-color, #f59e0b);
                    color: #fff;
                    border-color: var(--warning-color, #f59e0b);
                }
                .btn-danger {
                    background: var(--danger-color, #ef4444);
                    color: #fff;
                }
                .btn-danger:hover {
                    opacity: 0.85;
                    transform: translateY(-1px);
                }
                .btn-secondary {
                    background: var(--bg-secondary, #e2e8f0);
                    color: var(--text-primary, #1e293b);
                    border: 1px solid var(--border-color, #cbd5e1);
                }
                .btn-secondary:hover {
                    background: var(--border-color, #d1d5db);
                }
                .search-box {
                    padding: 6px 12px;
                    border: 1px solid var(--border-color, #e2e8f0);
                    border-radius: 6px;
                    background: var(--bg-primary, #fff);
                    color: var(--text-primary);
                    font-size: 13px;
                    min-width: 160px;
                }
                .sort-select {
                    padding: 6px 10px;
                    border: 1px solid var(--border-color, #e2e8f0);
                    border-radius: 6px;
                    background: var(--bg-primary, #fff);
                    color: var(--text-primary);
                    font-size: 13px;
                    cursor: pointer;
                }
                .sort-dir-btn {
                    background: var(--bg-secondary, #f1f5f9);
                    border: 1px solid var(--border-color, #e2e8f0);
                    border-radius: 6px;
                    padding: 6px 10px;
                    cursor: pointer;
                    font-size: 13px;
                    color: var(--text-primary);
                    display: inline-flex;
                    align-items: center;
                    gap: 4px;
                }
                .sort-dir-btn svg {
                    transition: transform 0.2s;
                    width: 14px;
                    height: 14px;
                }
                .conn-grid {
                    display: grid;
                    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
                    gap: 16px;
                    margin-top: 8px;
                }
                .conn-card {
                    background: var(--card-bg, #fff);
                    border: 1px solid var(--border-color, #e2e8f0);
                    border-radius: 10px;
                    padding: 14px 16px;
                    box-shadow: 0 1px 3px rgba(0,0,0,0.04);
                    transition: box-shadow 0.2s, transform 0.1s;
                    display: flex;
                    flex-direction: column;
                }
                .conn-card:hover {
                    box-shadow: 0 4px 12px rgba(0,0,0,0.07);
                    transform: translateY(-2px);
                }
                .conn-card.closed-card {
                    opacity: 0.75;
                    border-color: var(--border-color, #d1d5db);
                }
                .conn-card-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    margin-bottom: 10px;
                    border-bottom: 1px solid var(--border-color, #eef2f6);
                    padding-bottom: 8px;
                }
                .conn-host {
                    font-weight: 600;
                    font-size: 15px;
                    color: var(--text-primary, #1e293b);
                    overflow: hidden;
                    text-overflow: ellipsis;
                    white-space: nowrap;
                    flex: 1;
                }
                .conn-port {
                    font-size: 13px;
                    color: var(--text-secondary, #64748b);
                    margin-left: 4px;
                    margin-right: 8px;
                }
                .close-conn-btn {
                    background: var(--danger-color, #ef4444);
                    color: #fff;
                    border: none;
                    border-radius: 4px;
                    padding: 4px 10px;
                    font-size: 12px;
                    cursor: pointer;
                    transition: background 0.2s;
                }
                .close-conn-btn:hover {
                    background: #dc2626;
                }
                .clear-conn-btn {
                    background: transparent;
                    color: var(--text-secondary, #94a3b8);
                    border: none;
                    border-radius: 4px;
                    padding: 2px 6px;
                    font-size: 16px;
                    cursor: pointer;
                    transition: color 0.2s, background 0.2s;
                }
                .clear-conn-btn:hover {
                    color: var(--danger-color, #ef4444);
                    background: rgba(239,68,68,0.1);
                }
                .conn-card-body {
                    display: flex;
                    flex-direction: column;
                    gap: 6px;
                    font-size: 13px;
                }
                .conn-detail {
                    display: flex;
                    justify-content: space-between;
                    align-items: baseline;
                }
                .conn-detail .label {
                    color: var(--text-secondary, #64748b);
                    font-weight: 500;
                }
                .conn-detail span:last-child {
                    color: var(--text-primary, #1e293b);
                    word-break: break-all;
                    text-align: right;
                    max-width: 65%;
                }
                .empty-state {
                    grid-column: 1 / -1;
                    text-align: center;
                    padding: 60px 20px;
                    color: var(--text-secondary, #94a3b8);
                    font-size: 15px;
                    background: var(--bg-secondary, #f8fafc);
                    border-radius: 12px;
                    border: 1px dashed var(--border-color, #e2e8f0);
                }
                @media (max-width: 480px) {
                    .conn-grid { grid-template-columns: 1fr; }
                    .conn-toolbar { flex-direction: column; align-items: stretch; }
                    .conn-actions { flex-wrap: wrap; }
                }
            </style>
            <div class="card">
                <div class="conn-toolbar">
                    <div style="display:flex; align-items:center; gap:12px;">
                        <h3 style="margin:0;">${t('connections.title')}</h3>
                    </div>
                    <div class="conn-actions">
                        <div class="conn-tabs">
                            <button id="tab-active" class="conn-tab active">${t('connections.active')} (0)</button>
                            <button id="tab-closed" class="conn-tab">${t('connections.closed')} (0)</button>
                        </div>
                        <input type="text" id="conn-filter" class="search-box" placeholder="${t('connections.search_placeholder')}">
                        <select id="sort-select" class="sort-select">
                            <option value="host">${t('connections.host')}</option>
                            <option value="port">${t('connections.port')}</option>
                            <option value="rule">${t('connections.rule')}</option>
                            <option value="chain">${t('connections.chain')}</option>
                            <option value="uploadSpeed">${t('connections.upload_speed')}</option>
                            <option value="downloadSpeed">${t('connections.download_speed')}</option>
                        </select>
                        <button id="sort-dir-btn" class="sort-dir-btn">
                            <span class="sort-label">${t('connections.sort')}</span>
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <polyline points="6 9 12 15 18 9"/>
                            </svg>
                        </button>
                        <button id="toggle-pause" class="btn btn-pause">${t('connections.pause')}</button>
                        <button id="close-all-connections" class="btn btn-danger">${t('connections.close_all')}</button>
                        <button id="clear-all-closed" class="btn btn-secondary" style="display:none;">${t('connections.clear_all_closed') || '清空全部'}</button>
                    </div>
                </div>
                <div id="conn-grid" class="conn-grid">
                    <div class="empty-state">${t('connections.empty')}</div>
                </div>
            </div>
        `;

        document.getElementById('tab-active').addEventListener('click', () => {
            currentTab = 'active';
            document.querySelectorAll('.conn-tab').forEach(t => t.classList.remove('active'));
            document.getElementById('tab-active').classList.add('active');
            renderCards();
        });
        document.getElementById('tab-closed').addEventListener('click', () => {
            currentTab = 'closed';
            document.querySelectorAll('.conn-tab').forEach(t => t.classList.remove('active'));
            document.getElementById('tab-closed').classList.add('active');
            renderCards();
        });

        document.getElementById('conn-filter').addEventListener('input', (e) => {
            filterText = e.target.value.trim();
            scheduleRender(true);
        });
        const sortSelect = document.getElementById('sort-select');
        sortSelect.value = sortBy;
        sortSelect.addEventListener('change', () => setSort(sortSelect.value));
        document.getElementById('sort-dir-btn').addEventListener('click', () => {
            sortAsc = !sortAsc;
            updateSortButton();
            scheduleRender(true);
        });
        document.getElementById('toggle-pause').addEventListener('click', () => {
            isPaused = !isPaused;
            if (!isPaused) scheduleRender(true);
            else renderCards();
        });
        document.getElementById('close-all-connections').addEventListener('click', closeAllConnections);
        document.getElementById('clear-all-closed').addEventListener('click', clearAllClosed);

        updateSortButton();

        startWebSocket();
        renderCards();
        initLanguageListener();
    }

    async function init() {
        container = document.getElementById('connections-content');
        if (!container) return;
        render();
    }

    function destroy() {
        if (wsConn) wsConn.close();
        if (reconnectTimer) clearTimeout(reconnectTimer);
        if (pendingRenderTimer) clearTimeout(pendingRenderTimer);
        if (rafPending) cancelAnimationFrame(rafPending);
        activeConnMap.clear();
        closedConnMap.clear();
        prevSnapshot.clear();
        rafPending = false; isPaused = false;
        reconnectDelay = 1000; lastRenderTime = 0;
        if (langEventListener) {
            window.removeEventListener('languageChanged', langEventListener);
            langEventListener = null;
        }
    }

    return { init, destroy };
})();