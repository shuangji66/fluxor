window.Logs = (function() {
    let container = null;
    let logContainer = null;
    let wsLog = null;
    
    let logBuffer = [];
    let currentLevel = 'info';
    let paused = false;
    let autoScroll = true;
    let searchText = ''; // 新增搜索文本
    
    let reconnectTimer = null;
    let reconnectDelay = 1000;
    const MAX_RECONNECT_DELAY = 30000;
    
    let rafPending = false;
    const MAX_BUFFER_SIZE = 5000;
    const MAX_DOM_NODES = 1000;

    const LEVELS = ['debug', 'info', 'warning', 'error'];

    function t(key) {
        return (window.i18n && window.i18n.t) ? window.i18n.t(key) : key;
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/[&<>"']/g, m => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        })[m]);
    }

    function formatTime(date) {
        const pad = n => String(n).padStart(2, '0');
        return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${String(date.getMilliseconds()).padStart(3, '0')}`;
    }

    // 应用搜索过滤（基于当前 DOM 元素）
    function applySearchFilter() {
        if (!logContainer) return;
        const entries = logContainer.querySelectorAll('.log-entry');
        const lowerSearch = searchText.toLowerCase();
        for (const entry of entries) {
            const msgEl = entry.querySelector('.log-msg');
            if (msgEl) {
                const msgText = msgEl.textContent.toLowerCase();
                entry.style.display = msgText.includes(lowerSearch) ? '' : 'none';
            }
        }
    }

    function scheduleRender() {
        if (rafPending || !logContainer) return;
        rafPending = true;
        requestAnimationFrame(() => {
            rafPending = false;
            flushLogs();
        });
    }

    function flushLogs() {
        if (!logContainer) return;
        
        const curIdx = LEVELS.indexOf(currentLevel);
        const filtered = logBuffer.filter(l => LEVELS.indexOf(l.level) >= curIdx);
        
        const fragment = document.createDocumentFragment();
        const renderedCount = logContainer.childElementCount;
        const toAppend = filtered.slice(renderedCount);
        
        for (const log of toAppend) {
            const div = document.createElement('div');
            div.className = `log-entry log-${log.level}`;
            div.innerHTML = `<span class="log-time">${log.time}</span><span class="log-badge badge-${log.level}">${log.level.toUpperCase()}</span><span class="log-msg">${escapeHtml(log.msg)}</span>`;
            fragment.appendChild(div);
        }
        
        if (fragment.childNodes.length > 0) {
            logContainer.appendChild(fragment);
        }
        
        while (logContainer.childElementCount > MAX_DOM_NODES) {
            logContainer.removeChild(logContainer.firstChild);
        }
        
        // 应用搜索过滤
        applySearchFilter();
        
        if (!paused && autoScroll) {
            logContainer.scrollTop = logContainer.scrollHeight;
        }
    }

    function fullRender() {
        if (!logContainer) return;
        logContainer.innerHTML = '';
        const curIdx = LEVELS.indexOf(currentLevel);
        const filtered = logBuffer.filter(l => LEVELS.indexOf(l.level) >= curIdx);
        const fragment = document.createDocumentFragment();
        for (const log of filtered) {
            const div = document.createElement('div');
            div.className = `log-entry log-${log.level}`;
            div.innerHTML = `<span class="log-time">${log.time}</span><span class="log-badge badge-${log.level}">${log.level.toUpperCase()}</span><span class="log-msg">${escapeHtml(log.msg)}</span>`;
            fragment.appendChild(div);
        }
        logContainer.appendChild(fragment);
        applySearchFilter();
        if (!paused && autoScroll) {
            logContainer.scrollTop = logContainer.scrollHeight;
        }
    }

    function appendLog(level, message) {
        const entry = {
            level: level.toLowerCase(),
            msg: message,
            time: formatTime(new Date()),
            ts: Date.now()
        };
        
        logBuffer.push(entry);
        if (logBuffer.length > MAX_BUFFER_SIZE) {
            logBuffer.splice(0, logBuffer.length - MAX_BUFFER_SIZE);
            fullRender();
            return;
        }
        
        scheduleRender();
    }

    function initScrollDetection() {
        if (!logContainer) return;
        logContainer.addEventListener('scroll', () => {
            const threshold = 50;
            const atBottom = logContainer.scrollHeight - logContainer.scrollTop - logContainer.clientHeight < threshold;
            autoScroll = atBottom;
        }, { passive: true });
    }

    function connectWebSocket() {
        if (wsLog) { wsLog.close(); wsLog = null; }
        
        wsLog = window.API.wsConnect('/logs', (e) => {
            try {
                const data = JSON.parse(e.data);
                let level = data.type || data.level || 'info';
                let msg = data.payload || data.msg || data.message || JSON.stringify(data);
                appendLog(level, msg);
                reconnectDelay = 1000;
            } catch (err) {
                appendLog('info', e.data);
            }
        }, {
            onOpen: () => { 
                reconnectDelay = 1000; 
                appendLog('debug', t('logs.ws_connected'));
            },
            onClose: () => {
                if (reconnectTimer) clearTimeout(reconnectTimer);
                reconnectTimer = setTimeout(connectWebSocket, reconnectDelay);
                reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY);
            },
            onError: () => {}
        });
    }

    function setLevel(level) {
        if (level === currentLevel) return;
        currentLevel = level;
        
        document.querySelectorAll('.log-level-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.level === level);
        });
        
        fullRender();
    }

    function clearLogs() {
        logBuffer = [];
        if (logContainer) logContainer.innerHTML = '';
    }

    function togglePause() {
        paused = !paused;
        const btn = document.getElementById('log-pause-btn');
        if (btn) {
            btn.textContent = paused ? t('logs.resume') : t('logs.pause');
            btn.classList.toggle('paused', paused);
        }
        if (!paused) {
            const threshold = 50;
            const atBottom = logContainer.scrollHeight - logContainer.scrollTop - logContainer.clientHeight < threshold;
            if (atBottom) {
                autoScroll = true;
                logContainer.scrollTop = logContainer.scrollHeight;
            }
        }
    }

    function onSearchInput(e) {
        searchText = e.target.value.trim();
        applySearchFilter();
    }

    function render() {
        if (!container) return;
        container.innerHTML = `
            <style>
                .log-controls { display:flex; flex-wrap:wrap; gap:10px; margin-bottom:12px; align-items:center; }
                .log-controls-left { display:flex; flex-wrap:wrap; gap:6px; flex:1; }
                .log-controls-right { display:flex; flex-wrap:wrap; gap:6px; }
                .log-level-btn { padding:4px 12px; border:1px solid var(--border-color,#e2e8f0); border-radius:6px; background:var(--bg-secondary,#f8fafc); color:var(--text-secondary,#64748b); cursor:pointer; font-size:12px; font-weight:600; transition:all 0.2s; }
                .log-level-btn.active { background:var(--primary-color,#3b82f6); color:#fff; border-color:var(--primary-color,#3b82f6); }
                .log-action { padding:4px 12px; border:1px solid var(--border-color,#e2e8f0); border-radius:6px; background:var(--bg-secondary,#f8fafc); color:var(--text-primary,#1e293b); cursor:pointer; font-size:12px; transition:all 0.2s; }
                .log-action.paused { background:var(--warning-color,#f59e0b); color:#fff; border-color:var(--warning-color,#f59e0b); }
                .log-action-danger { background:var(--danger,#ef4444); color:#fff; border-color:var(--danger,#ef4444); }
                .log-action-danger:hover { opacity:0.85; }
                .log-search { padding:4px 10px; border:1px solid var(--border-color,#e2e8f0); border-radius:6px; background:var(--bg-input,#fff); color:var(--text-primary); font-size:12px; min-width:150px; }
                .log-search:focus { outline:none; border-color:var(--accent); }
                .log-container { height:calc(100vh - 280px); min-height:300px; overflow-y:auto; background:var(--bg-secondary); border:1px solid var(--border-color); border-radius:8px; padding:12px; font-family:system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif; font-size:13px; line-height:1.6; scroll-behavior:auto; }
                .log-entry { display:flex; gap:8px; align-items:baseline; word-break:break-all; }
                .log-time { color:var(--text-secondary); flex-shrink:0; font-size:12px; }
                .log-badge { padding:0 6px; border-radius:3px; font-size:11px; font-weight:700; flex-shrink:0; min-width:56px; text-align:center; }
                .badge-debug { background:var(--border-color); color:var(--text-secondary); }
                .badge-info { background:var(--accent); color:#fff; }
                .badge-warning { background:var(--warning); color:#000; }
                .badge-error { background:var(--danger); color:#fff; }
                .log-msg { color:var(--text-primary); white-space:pre-wrap; }
                .log-entry[style*="display: none"] { display: none !important; }
            </style>
            <div class="card">
                <h3>${t('logs.title')}</h3>
                <div class="log-controls">
                    <div class="log-controls-left">
                        ${LEVELS.map(l => `<button class="log-level-btn${l === currentLevel ? ' active' : ''}" data-level="${l}">${l.toUpperCase()}</button>`).join('')}
                        <input type="text" id="log-search" class="log-search" placeholder="${t('logs.search') || '搜索...'}">
                    </div>
                    <div class="log-controls-right">
                        <button id="log-pause-btn" class="log-action${paused ? ' paused' : ''}">${paused ? t('logs.resume') : t('logs.pause')}</button>
                        <button id="log-clear-btn" class="log-action log-action-danger">${t('logs.clear')}</button>
                    </div>
                </div>
                <div class="log-container" id="log-viewer"></div>
            </div>
        `;
        
        logContainer = document.getElementById('log-viewer');
        initScrollDetection();
        
        document.querySelectorAll('.log-level-btn').forEach(btn => {
            btn.addEventListener('click', () => setLevel(btn.dataset.level));
        });
        document.getElementById('log-pause-btn').addEventListener('click', togglePause);
        document.getElementById('log-clear-btn').addEventListener('click', clearLogs);
        
        // 搜索框事件
        const searchInput = document.getElementById('log-search');
        if (searchInput) {
            searchInput.addEventListener('input', onSearchInput);
        }
        
        fullRender();
        connectWebSocket();
    }

    async function init() {
        container = document.getElementById('logs-content');
        if (!container) return;
        render();
    }

    function destroy() {
        if (wsLog) wsLog.close();
        if (reconnectTimer) clearTimeout(reconnectTimer);
        logContainer = null;
        if (rafPending) { cancelAnimationFrame(rafPending); rafPending = false; }
    }

    return { init, destroy };
})();