// 规则模块：虚拟滚动 + 并发提供商更新 + 搜索过滤（支持中英文）
window.Rules = (function() {
    let container = null;
    let allRules = [];
    let providers = {};
    let filterText = '';
    let langEventListener = null;

    // 虚拟滚动状态
    const ROW_HEIGHT = 40;
    const BUFFER_SIZE = 10;
    let visibleStart = 0;
    let visibleEnd = 0;
    let filteredRules = [];
    let listContainer = null;
    let scrollSpacer = null;
    let viewport = null;

    function t(key) {
        return (window.i18n && window.i18n.t) ? window.i18n.t(key) : key;
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/[&<>"']/g, m => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        })[m]);
    }

    // ==================== 数据获取 ====================
    async function fetchRules() {
        try {
            const resp = await window.API.apiFetch('/rules');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            allRules = data.rules || [];
            console.log('[Rules] 加载规则数:', allRules.length);
            applyFilter();
        } catch (err) {
            console.error('[Rules] 加载规则失败:', err);
            showError(t('rules.load_failed') + ': ' + err.message);
        }
    }

    async function fetchProviders() {
        try {
            const resp = await window.API.apiFetch('/providers/rules');
            if (!resp.ok) return;
            const data = await resp.json();
            providers = data.providers || {};
            console.log('[Rules] 加载提供商数:', Object.keys(providers).length);
            renderProviders();
        } catch (err) {
            console.warn('[Rules] 规则提供商加载失败', err);
        }
    }

    // ==================== 过滤与虚拟滚动核心 ====================
    function applyFilter() {
        if (!filterText) {
            filteredRules = allRules;
        } else {
            const lower = filterText.toLowerCase();
            filteredRules = allRules.filter(rule =>
                rule.type?.toLowerCase().includes(lower) ||
                rule.payload?.toLowerCase().includes(lower) ||
                rule.proxy?.toLowerCase().includes(lower)
            );
        }
        const countEl = document.getElementById('rules-count');
        if (countEl) countEl.innerText = filteredRules.length;
        if (viewport) viewport.scrollTop = 0;
        updateVisibleRange();
    }

    function updateVisibleRange() {
        if (!viewport || !scrollSpacer || !listContainer) return;

        const scrollTop = viewport.scrollTop;
        const viewHeight = viewport.clientHeight;
        const totalHeight = filteredRules.length * ROW_HEIGHT;

        scrollSpacer.style.height = totalHeight + 'px';

        const newStart = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - BUFFER_SIZE);
        const newEnd = Math.min(filteredRules.length, Math.ceil((scrollTop + viewHeight) / ROW_HEIGHT) + BUFFER_SIZE);

        if (newStart !== visibleStart || newEnd !== visibleEnd) {
            visibleStart = newStart;
            visibleEnd = newEnd;
            renderVisibleRows();
        }
    }

    function renderVisibleRows() {
        if (!listContainer) return;
        const fragment = document.createDocumentFragment();

        for (let i = visibleStart; i < visibleEnd; i++) {
            const rule = filteredRules[i];
            const tr = document.createElement('tr');
            tr.style.height = ROW_HEIGHT + 'px';
            tr.innerHTML = `
                <td class="rule-type"><span class="type-badge" title="${escapeHtml(rule.type || '-')}">${escapeHtml(rule.type || '-')}</span></td>
                <td class="rule-payload" title="${escapeHtml(rule.payload || '')}">${escapeHtml(rule.payload || '-')}</td>
                <td class="rule-proxy" title="${escapeHtml(rule.proxy || '')}">${escapeHtml(rule.proxy || '-')}</td>
            `;
            fragment.appendChild(tr);
        }

        listContainer.innerHTML = '';
        listContainer.style.paddingTop = (visibleStart * ROW_HEIGHT) + 'px';
        listContainer.appendChild(fragment);
    }

    // ==================== 提供商管理（并发更新） ====================
    async function updateProvider(name, btn) {
        if (btn) { btn.disabled = true; btn.textContent = '⏳'; }
        try {
            const resp = await window.API.apiFetch(`/providers/rules/${encodeURIComponent(name)}`, { method: 'PUT' });
            if (!resp.ok) throw new Error(await resp.text());
            showToast(t('rules.provider_update_success').replace('{name}', name), 'success');
            await fetchProviders();
        } catch (err) {
            console.error('[Rules] 更新提供商失败:', name, err);
            showToast(t('rules.provider_update_failed').replace('{name}', name) + ': ' + err.message, 'error');
        } finally {
            if (btn) { btn.disabled = false; btn.textContent = '🔄'; }
        }
    }

    async function updateAllProviders() {
        const names = Object.keys(providers);
        if (names.length === 0) {
            showToast(t('rules.no_providers'), 'info');
            return;
        }
        showToast(t('rules.updating_providers').replace('{count}', names.length), 'info');
        const btns = document.querySelectorAll('.update-provider');
        const btnMap = {};
        btns.forEach(b => btnMap[b.dataset.provider] = b);

        const concurrency = 5;
        for (let i = 0; i < names.length; i += concurrency) {
            const batch = names.slice(i, i + concurrency);
            await Promise.allSettled(
                batch.map(name => updateProvider(name, btnMap[name]))
            );
        }
        showToast(t('rules.batch_update_complete'), 'success');
    }

    // ==================== UI 渲染 ====================
    function renderProviders() {
        const el = document.getElementById('providers-list');
        if (!el) return;
        const entries = Object.entries(providers);
        if (entries.length === 0) {
            el.innerHTML = `<div class="empty-state"><p>${t('rules.no_providers')}</p></div>`;
            return;
        }
        el.innerHTML = entries.map(([name, p]) => {
            const count = p.ruleCount ?? p.rules?.length ?? 0;
            const time = p.updatedAt ? new Date(p.updatedAt).toLocaleString() : t('rules.unknown_time');
            const behavior = p.behavior ? ` · ${p.behavior}` : '';
            return `
                <div class="provider-card">
                    <div class="provider-info">
                        <div class="provider-header">
                            <strong title="${escapeHtml(name)}">${escapeHtml(name)}</strong>
                            <button class="update-provider" data-provider="${escapeHtml(name)}" title="${t('rules.update_provider')}">🔄</button>
                        </div>
                        <div class="provider-meta">
                            <span class="meta-item">📋 ${count} ${t('rules.rules_count')}</span>
                            ${behavior ? `<span class="meta-item">🏷️ ${behavior}</span>` : ''}
                        </div>
                        <div class="provider-time">⏱️ ${time}</div>
                    </div>
                </div>`;
        }).join('');

        el.querySelectorAll('.update-provider').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                updateProvider(btn.dataset.provider, btn);
            });
        });
    }

    function initVirtualScroll() {
        viewport = document.getElementById('rules-viewport');
        listContainer = document.getElementById('rules-tbody');
        scrollSpacer = document.getElementById('rules-spacer');
        if (!viewport) return;

        viewport.addEventListener('scroll', updateVisibleRange, { passive: true });
        updateVisibleRange();
    }

    function showToast(msg, type) {
        let toast = document.getElementById('rules-toast');
        if (!toast) {
            toast = document.createElement('div');
            toast.id = 'rules-toast';
            toast.style.cssText = 'position:fixed;top:20px;right:20px;z-index:9999;padding:12px 20px;border-radius:8px;font-size:14px;color:#fff;transition:opacity 0.3s;pointer-events:none;';
            document.body.appendChild(toast);
        }
        toast.textContent = msg;
        toast.style.background = type === 'error' ? '#ef4444' : type === 'success' ? '#22c55e' : '#3b82f6';
        toast.style.opacity = '1';
        clearTimeout(toast._timer);
        toast._timer = setTimeout(() => { toast.style.opacity = '0'; }, 2500);
    }

    function showError(msg) {
        if (!allRules.length) {
            const c = document.getElementById('rules-content');
            if (c) c.innerHTML = `<div class="card error-card"><div style="padding:40px;text-align:center;color:#dc2626;">${escapeHtml(msg)}</div></div>`;
        } else showToast(msg, 'error');
    }

    // ==================== 语言切换刷新 ====================
    function refreshUI() {
        renderProviders();
        const countEl = document.getElementById('rules-count');
        if (countEl) countEl.innerText = filteredRules.length;
        const filterInput = document.getElementById('rule-filter');
        if (filterInput && window.i18n) {
            filterInput.placeholder = window.i18n.t('rules.search_placeholder');
        }
        const updateAllBtn = document.getElementById('update-all-providers');
        if (updateAllBtn && window.i18n) {
            updateAllBtn.textContent = '🔄 ' + window.i18n.t('rules.update_all_btn');
        }
        const providersTitle = container?.querySelector('.providers-section h3');
        if (providersTitle && window.i18n) {
            providersTitle.textContent = window.i18n.t('rules.providers_title');
        }
    }

    function onLanguageChange() {
        refreshUI();
    }

    function initLanguageListener() {
        if (langEventListener) {
            window.removeEventListener('languageChanged', langEventListener);
        }
        langEventListener = onLanguageChange;
        window.addEventListener('languageChanged', langEventListener);
    }

    // ==================== 入口与销毁 ====================
    function render() {
        if (!container) return;
        container.innerHTML = `
            <style>
                .rules-toolbar{display:flex;gap:8px;margin-bottom:16px;flex-wrap:wrap;align-items:center}
                .search-box{flex:1;min-width:200px;padding:8px 12px;border:1px solid var(--border-color,#e2e8f0);border-radius:6px;background:var(--bg-primary,#fff);color:var(--text-primary);font-size:13px;transition:all .2s}
                .search-box:focus{outline:none;border-color:var(--accent,#3b82f6);box-shadow:0 0 0 2px rgba(59,130,246,0.1)}
                .rules-table-wrapper{border:1px solid var(--border-color,#e2e8f0);border-radius:8px;overflow:hidden}
                .rules-table{width:100%;border-collapse:collapse;background:var(--card-bg,#fff)}
                .rules-table thead{background:var(--bg-secondary,#f8fafc);border-bottom:1px solid var(--border-color,#e2e8f0)}
                .rules-table th{padding:10px 12px;text-align:left;font-weight:600;color:var(--text-primary,#1e293b);font-size:12px}
                .rules-viewport{height:400px;overflow-y:auto;overflow-x:hidden;background:var(--card-bg,#fff)}
                .rules-viewport tbody{display:block}
                .rules-viewport tr{display:table;width:100%;table-layout:fixed;border-bottom:1px solid var(--border-color,#e2e8f0);transition:background .1s}
                .rules-viewport tr:hover{background:var(--hover-bg,#f8fafc)}
                .rules-viewport td{padding:8px 12px;font-size:12px;color:var(--text-primary,#1e293b);word-break:break-all}
                .type-badge{display:inline-block;padding:2px 8px;border-radius:4px;background:var(--bg-primary,#e2e8f0);color:var(--text-secondary,#64748b);font-weight:600;font-size:11px}
                .rule-type{width:120px;flex-shrink:0}
                .rule-payload{flex:1;min-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
                .rule-proxy{width:150px;flex-shrink:0;font-weight:500;color:var(--accent,#3b82f6)}
                .rules-footer{padding:12px 16px;background:var(--bg-secondary,#f8fafc);border-top:1px solid var(--border-color,#e2e8f0);font-size:12px;color:var(--text-secondary,#64748b);text-align:center}
                .empty-state{padding:60px 20px;text-align:center;color:var(--text-secondary,#64748b)}
                .provider-card{display:flex;justify-content:space-between;align-items:flex-start;padding:12px 14px;border:1px solid var(--border-color,#e2e8f0);border-radius:8px;margin-bottom:8px;background:var(--card-bg,#fff);transition:all .2s}
                .provider-card:hover{border-color:var(--accent,#3b82f6);box-shadow:0 1px 3px rgba(59,130,246,0.1)}
                .provider-info{flex:1;overflow:hidden}
                .provider-header{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:6px}
                .provider-header strong{color:var(--text-primary,#1e293b);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1}
                .update-provider{padding:4px 10px;border:1px solid var(--border-color,#e2e8f0);border-radius:4px;background:transparent;cursor:pointer;font-size:12px;transition:all .15s;flex-shrink:0}
                .update-provider:hover:not(:disabled){border-color:var(--accent,#3b82f6);background:rgba(59,130,246,0.08)}
                .update-provider:disabled{opacity:.5;cursor:not-allowed}
                .provider-meta{display:flex;gap:12px;flex-wrap:wrap;margin-bottom:4px;font-size:12px;color:var(--text-secondary,#64748b)}
                .meta-item{display:flex;align-items:center;gap:4px}
                .provider-time{font-size:11px;color:var(--text-secondary,#94a3b8)}
                .btn-sm{padding:6px 14px;border:1px solid var(--border-color,#e2e8f0);border-radius:6px;background:var(--bg-secondary,#f8fafc);color:var(--text-primary,#1e293b);cursor:pointer;font-size:12px;font-weight:600;transition:all .2s;display:inline-flex;align-items:center;gap:4px}
                .btn-sm:hover{background:var(--accent,#3b82f6);color:#fff;border-color:var(--accent,#3b82f6);transform:translateY(-1px)}
                .btn-sm:disabled{opacity:.5;cursor:not-allowed;transform:none}
                .card h3{margin:0 0 12px 0}
                .providers-section{margin-top:20px}
            </style>
            <div class="card">
                <h3>${t('rules.title')}</h3>
                <div class="rules-toolbar">
                    <input type="text" id="rule-filter" class="search-box" placeholder="${t('rules.search_placeholder')}" value="${escapeHtml(filterText)}">
                    <button id="update-all-providers" class="btn-sm">🔄 ${t('rules.update_all_btn')}</button>
                </div>
                <div class="rules-table-wrapper">
                    <table class="rules-table">
                        <thead><tr><th class="rule-type">${t('rules.type')}</th><th>${t('rules.payload')}</th><th class="rule-proxy">${t('rules.proxy')}</th></tr></thead>
                    </table>
                    <div id="rules-viewport" class="rules-viewport">
                        <div id="rules-spacer"></div>
                        <tbody id="rules-tbody"></tbody>
                    </div>
                </div>
                <div class="rules-footer">${t('rules.total')} <strong id="rules-count">0</strong> ${t('rules.rules_count')}</div>
            </div>
            <div class="card providers-section">
                <h3>${t('rules.providers_title')}</h3>
                <div id="providers-list"></div>
            </div>
        `;

        const filterInput = document.getElementById('rule-filter');
        if (filterInput) {
            filterInput.addEventListener('input', e => {
                filterText = e.target.value.trim();
                applyFilter();
            });
        }
        const updateAllBtn = document.getElementById('update-all-providers');
        if (updateAllBtn) updateAllBtn.addEventListener('click', updateAllProviders);

        initVirtualScroll();
        fetchRules();
        fetchProviders();
        initLanguageListener();
    }

    async function init() {
        container = document.getElementById('rules-content');
        if (!container) return;
        console.log('[Rules] 初始化模块');
        render();
    }

    function destroy() {
        allRules = [];
        filteredRules = [];
        providers = {};
        visibleStart = visibleEnd = 0;
        listContainer = scrollSpacer = viewport = null;
        if (container) container.innerHTML = '';
        if (langEventListener) {
            window.removeEventListener('languageChanged', langEventListener);
            langEventListener = null;
        }
        console.log('[Rules] 销毁模块');
    }

    return { init, destroy };
})();
