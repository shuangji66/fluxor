// 规则模块：规则列表和规则提供商管理（支持切换视图）
window.Rules = (function() {
    let container = null;
    let allRules = [];
    let allProviders = [];
    let filterText = '';
    let langEventListener = null;
    let currentView = 'rules'; // 'rules' | 'providers'

    const BASE = window.BASE_URL || '';
    function withBase(path) {
        if (!BASE || BASE === '/') return path;
        return BASE.replace(/\/$/, '') + '/' + path.replace(/^\//, '');
    }

    function t(key) {
        return (window.i18n && window.i18n.t) ? window.i18n.t(key) : key;
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/[&<>"']/g, m => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        })[m]);
    }

    // 格式化时间
    function formatTime(isoString) {
        if (!isoString) return '-';
        try {
            const date = new Date(isoString);
            if (isNaN(date.getTime())) return isoString;
            return date.toLocaleString();
        } catch (_) {
            return isoString;
        }
    }

    // ==================== 数据获取 ====================
    async function fetchProviders() {
        try {
            const resp = await window.API.apiFetch('/providers/rules');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            const providersObj = data.providers || {};
            allProviders = Object.keys(providersObj).map(name => ({
                name: name,
                ...providersObj[name]
            }));
        } catch (err) {
            console.error('[Rules] 加载提供商失败:', err);
            allProviders = [];
        }
        if (currentView === 'providers') {
            renderProvidersTable();
            // 更新计数
            updateProviderCount();
        }
    }

    async function fetchRules() {
        try {
            const resp = await window.API.apiFetch('/rules');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            allRules = data.rules || [];
            allRules.forEach((rule, idx) => {
                rule.index = idx;
                rule.enabled = !(rule.extra?.disabled === true);
            });
            if (currentView === 'rules') renderRules();
        } catch (err) {
            console.error('[Rules] 加载规则失败:', err);
            showToast(t('rules.load_failed') + ': ' + err.message, 'error');
            allRules = [];
            if (currentView === 'rules') renderRules();
        }
    }

    // ==================== 更新单个提供商 ====================
    async function updateProvider(providerName) {
        if (!providerName) {
            showToast('提供商名称无效', 'error');
            return;
        }

        const toastMsg = (key, fallback) => t(key) || fallback;
        showToast(
            toastMsg('rules.updating_provider', '正在更新 {name}...').replace('{name}', providerName),
            'info'
        );

        try {
            const encodedName = encodeURIComponent(providerName);
            const url = withBase(`/providers/rules/${encodedName}`);
            const updateResp = await fetch(url, {
                method: 'PUT',
            });

            if (updateResp.ok) {
                showToast(
                    toastMsg('rules.provider_update_success', '{name} 更新成功').replace('{name}', providerName),
                    'success'
                );
                await fetchProviders();
                if (currentView === 'rules') await fetchRules();
            } else {
                let errMsg = `HTTP ${updateResp.status}`;
                try {
                    const errData = await updateResp.json();
                    errMsg = errData.message || errData.error || errMsg;
                } catch (_) {
                    const text = await updateResp.text();
                    if (text) errMsg = text;
                }
                console.error(`[Rules] 更新失败响应:`, errMsg);
                throw new Error(errMsg);
            }
        } catch (err) {
            console.error(`[Rules] 更新提供商 ${providerName} 失败:`, err);
            showToast(
                toastMsg('rules.provider_update_failed', '{name} 更新失败').replace('{name}', providerName) + ': ' + err.message,
                'error'
            );
        }
    }

    // ==================== 更新全部提供商 ====================
    async function updateAllProviders() {
        if (allProviders.length === 0) {
            showToast(t('rules.no_providers') || '没有规则提供商', 'info');
            return;
        }

        showToast(
            (t('rules.updating_providers') || '正在更新 {count} 个提供商...').replace('{count}', allProviders.length),
            'info'
        );

        let successCount = 0;
        let failCount = 0;
        const errors = [];

        const updatePromises = allProviders.map(async (provider) => {
            const providerName = provider.name;
            if (!providerName) {
                failCount++;
                errors.push('未知提供商名称');
                return;
            }

            try {
                const url = withBase(`/providers/rules/${encodeURIComponent(providerName)}`);
                const updateResp = await fetch(url, {
                    method: 'PUT',
                });

                if (updateResp.ok) {
                    successCount++;
                } else {
                    let errMsg = `HTTP ${updateResp.status}`;
                    try {
                        const errData = await updateResp.json();
                        errMsg = errData.message || errData.error || errMsg;
                    } catch (_) {
                        const text = await updateResp.text();
                        if (text) errMsg = text;
                    }
                    failCount++;
                    errors.push(`${providerName}: ${errMsg}`);
                }
            } catch (err) {
                failCount++;
                errors.push(`${providerName}: ${err.message}`);
            }
        });

        await Promise.all(updatePromises);

        if (failCount === 0) {
            showToast(
                (t('rules.batch_update_complete') || '批量更新完成') + `: ${successCount} 成功`,
                'success'
            );
        } else {
            const detail = errors.slice(0, 5).join('; ');
            const msg = (t('rules.batch_update_partial') || '更新完成: {success} 成功, {fail} 失败')
                .replace('{success}', successCount)
                .replace('{fail}', failCount);
            showToast(msg, 'warning');
            console.warn('[Rules] 更新详情:', detail);
        }

        await fetchProviders();
        if (currentView === 'rules') await fetchRules();
    }

    // ==================== 更新计数 ====================
    function updateProviderCount() {
        const countEl = document.getElementById('providers-count');
        if (countEl) countEl.textContent = allProviders.length;
    }

    // ==================== 渲染提供商表格 ====================
    function renderProvidersTable() {
        const tbody = document.getElementById('providers-tbody');
        if (!tbody) return;

        tbody.innerHTML = '';
        const fragment = document.createDocumentFragment();

        if (allProviders.length === 0) {
            const tr = document.createElement('tr');
            tr.innerHTML = `<td colspan="5" style="text-align:center;padding:40px;color:var(--text-secondary);">${t('rules.no_providers') || '没有规则提供商'}</td>`;
            fragment.appendChild(tr);
        } else {
            for (const provider of allProviders) {
                const tr = document.createElement('tr');
                const name = provider.name || 'Unknown';
                const type = provider.type || provider.behavior || '-';
                const ruleCount = provider.ruleCount || provider.rule_count || 0;
                const updatedAt = provider.updatedAt || provider.updated_at || '';
                const timeStr = formatTime(updatedAt);
                tr.innerHTML = `
                    <td class="provider-name-cell" title="${escapeHtml(name)}">${escapeHtml(name)}</td>
                    <td class="provider-type-cell">${escapeHtml(type)}</td>
                    <td class="provider-rule-count-cell">${ruleCount}</td>
                    <td class="provider-time-cell" title="${escapeHtml(updatedAt)}">${escapeHtml(timeStr)}</td>
                    <td class="provider-action-cell">
                        <button class="btn btn-icon provider-update-btn" data-provider="${encodeURIComponent(name)}" title="${t('rules.update_provider')}">
                            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <polyline points="23 4 23 10 17 10"/>
                                <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
                            </svg>
                        </button>
                    </td>
                `;
                fragment.appendChild(tr);
            }
        }

        tbody.appendChild(fragment);

        tbody.querySelectorAll('.provider-update-btn').forEach(btn => {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                const encodedName = this.dataset.provider;
                if (encodedName) {
                    const name = decodeURIComponent(encodedName);
                    updateProvider(name);
                }
            });
        });

        // 更新计数
        updateProviderCount();
    }

    // ==================== 渲染规则列表 ====================
    function renderRules() {
        const tbody = document.getElementById('rules-tbody');
        if (!tbody) return;

        let displayRules = allRules;
        if (filterText) {
            const lower = filterText.toLowerCase();
            displayRules = allRules.filter(rule =>
                (rule.payload || '').toLowerCase().includes(lower) ||
                (rule.proxy || '').toLowerCase().includes(lower)
            );
        }

        const countEl = document.getElementById('rules-count');
        if (countEl) countEl.innerText = displayRules.length;

        tbody.innerHTML = '';
        const fragment = document.createDocumentFragment();

        for (const rule of displayRules) {
            const tr = document.createElement('tr');
            const isEnabled = rule.enabled !== false;
            tr.innerHTML = `
                <td class="rule-enabled" style="text-align:center;width:60px;">
                    <label class="toggle-switch" style="width:36px;height:20px;">
                        <input type="checkbox" class="rule-toggle" data-index="${rule.index}" ${isEnabled ? 'checked' : ''}>
                        <span class="slider" style="height:20px;width:36px;"></span>
                    </label>
                </td>
                <td class="rule-payload" title="${escapeHtml(rule.payload || '')}">${escapeHtml(rule.payload || '-')}</td>
                <td class="rule-proxy" title="${escapeHtml(rule.proxy || '')}">${escapeHtml(rule.proxy || '-')}</td>
            `;
            const toggle = tr.querySelector('.rule-toggle');
            toggle.addEventListener('change', function(e) {
                e.stopPropagation();
                const index = parseInt(this.dataset.index);
                const ruleData = allRules.find(r => r.index === index);
                if (!ruleData) return;
                const oldEnabled = ruleData.enabled;
                toggleRule(index, oldEnabled);
            });

            fragment.appendChild(tr);
        }

        tbody.appendChild(fragment);
    }

    // ==================== 规则启用/禁用 ====================
    async function toggleRule(index, currentEnabled) {
        const rule = allRules.find(r => r.index === index);
        if (rule) {
            rule.enabled = !currentEnabled;
        }
        renderRules();

        try {
            const payload = { [index]: currentEnabled };
            const resp = await window.API.apiFetch('/rules/disable', {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
            if (!resp.ok) {
                throw new Error(`HTTP ${resp.status}`);
            }
            showToast(currentEnabled ? '规则已禁用' : '规则已启用', 'success');
        } catch (err) {
            console.error('[Rules] 切换规则状态失败:', err);
            if (rule) {
                rule.enabled = currentEnabled;
            }
            renderRules();
            showToast(t('common.operation_failed') + ': ' + err.message, 'error');
        }
    }

    // ==================== Toast 提示 ====================
    function showToast(msg, type = 'info') {
        let toast = document.getElementById('rules-toast');
        if (!toast) {
            toast = document.createElement('div');
            toast.id = 'rules-toast';
            toast.style.cssText = 'position:fixed;top:20px;right:20px;z-index:9999;padding:12px 20px;border-radius:8px;font-size:14px;color:#fff;transition:opacity 0.3s;pointer-events:none;';
            document.body.appendChild(toast);
        }
        toast.textContent = msg;
        toast.style.background = type === 'error' ? '#ef4444' : type === 'success' ? '#22c55e' : type === 'warning' ? '#f59e0b' : '#3b82f6';
        toast.style.opacity = '1';
        clearTimeout(toast._timer);
        toast._timer = setTimeout(() => { toast.style.opacity = '0'; }, 3500);
    }

    // ==================== 视图切换 ====================
    function switchView(view) {
        currentView = view;
        document.querySelectorAll('.view-tab').forEach(tab => {
            tab.classList.toggle('active', tab.dataset.view === view);
        });

        const rulesContainer = document.getElementById('rules-view');
        const providersContainer = document.getElementById('providers-view');

        if (view === 'rules') {
            rulesContainer.style.display = 'block';
            providersContainer.style.display = 'none';
            renderRules();
        } else {
            rulesContainer.style.display = 'none';
            providersContainer.style.display = 'block';
            renderProvidersTable();
        }
    }

    // ==================== 语言切换刷新 ====================
    function refreshUI() {
        const countEl = document.getElementById('rules-count');
        if (countEl) countEl.innerText = allRules.length;
        const filterInput = document.getElementById('rule-filter');
        if (filterInput && window.i18n) {
            filterInput.placeholder = window.i18n.t('rules.search_placeholder');
        }
        const titleEl = container?.querySelector('.rules-title');
        if (titleEl && window.i18n) {
            titleEl.textContent = window.i18n.t('rules.title');
        }
        const updateAllBtn = document.getElementById('update-all-rules');
        if (updateAllBtn && window.i18n) {
            updateAllBtn.textContent = window.i18n.t('rules.update_all');
        }
        if (currentView === 'rules') renderRules();
        else {
            renderProvidersTable();
            updateProviderCount();
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
                .rules-toolbar {
                    display: flex;
                    gap: 8px;
                    margin-bottom: 16px;
                    flex-wrap: wrap;
                    align-items: center;
                }
                .search-box {
                    flex: 1;
                    min-width: 180px;
                    padding: 8px 12px;
                    border: 1px solid var(--border-color, #e2e8f0);
                    border-radius: 6px;
                    background: var(--bg-input, #fff);
                    color: var(--text-primary, #1e293b);
                    font-size: 13px;
                    transition: all .2s;
                }
                .search-box:focus {
                    outline: none;
                    border-color: var(--accent, #3b82f6);
                    box-shadow: 0 0 0 2px rgba(59,130,246,0.1);
                }
                .view-tabs {
                    display: flex;
                    gap: 4px;
                    background: var(--bg-secondary, #f1f5f9);
                    padding: 4px;
                    border-radius: 8px;
                }
                .view-tab {
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
                .view-tab.active {
                    background: var(--accent, #3b82f6);
                    color: #fff;
                    font-weight: 600;
                    box-shadow: 0 2px 4px rgba(59,130,246,0.3);
                }
                .view-tab:hover:not(.active) {
                    background: rgba(255,255,255,0.5);
                }
                .rules-table-wrapper {
                    border: 1px solid var(--border-color, #e2e8f0);
                    border-radius: 8px;
                    overflow: auto;
                }
                .rules-table {
                    width: 100%;
                    border-collapse: collapse;
                    background: var(--card-bg, #fff);
                }
                .rules-table thead {
                    background: var(--bg-secondary, #f8fafc);
                    border-bottom: 1px solid var(--border-color, #e2e8f0);
                }
                .rules-table th {
                    padding: 8px 12px;
                    text-align: left;
                    font-weight: 600;
                    color: var(--text-primary, #1e293b);
                    font-size: 12px;
                }
                .rules-table th.rule-enabled {
                    text-align: center;
                    width: 60px;
                }
                .rules-table th.rule-payload {
                    text-align: left;
                }
                .rules-table th.rule-proxy {
                    text-align: left;
                }
                .rules-table tbody tr {
                    border-bottom: 1px solid var(--border-color, #e2e8f0);
                    transition: background .1s;
                }
                .rules-table tbody tr:hover {
                    background: var(--hover-bg, #f8fafc);
                }
                .rules-table td {
                    padding: 6px 10px;
                    font-size: 13px;
                    color: var(--text-primary, #1e293b);
                    word-break: break-all;
                }
                .rule-payload {
                    flex: 1;
                    min-width: 120px;
                    overflow: hidden;
                    text-overflow: ellipsis;
                    white-space: nowrap;
                }
                .rule-proxy {
                    width: 120px;
                    flex-shrink: 0;
                    font-weight: 500;
                    color: var(--accent, #3b82f6);
                }
                .rule-enabled {
                    width: 60px;
                    text-align: center;
                    flex-shrink: 0;
                }
                .rules-footer {
                    padding: 8px 12px;
                    background: var(--bg-secondary, #f8fafc);
                    border-top: 1px solid var(--border-color, #e2e8f0);
                    font-size: 13px;
                    color: var(--text-secondary, #64748b);
                    text-align: center;
                }
                .toggle-switch {
                    position: relative;
                    display: inline-block;
                    width: 36px;
                    height: 20px;
                    flex-shrink: 0;
                }
                .toggle-switch input {
                    opacity: 0;
                    width: 0;
                    height: 0;
                }
                .slider {
                    position: absolute;
                    cursor: pointer;
                    top: 0;
                    left: 0;
                    right: 0;
                    bottom: 0;
                    background-color: var(--border-color, #ccc);
                    transition: .3s;
                    border-radius: 20px;
                }
                .slider:before {
                    position: absolute;
                    content: "";
                    height: 14px;
                    width: 14px;
                    left: 3px;
                    bottom: 3px;
                    background-color: white;
                    transition: .3s;
                    border-radius: 50%;
                }
                input:checked + .slider {
                    background-color: var(--accent, #3b82f6);
                }
                input:checked + .slider:before {
                    transform: translateX(16px);
                }
                .btn {
                    padding: 6px 14px;
                    border-radius: 6px;
                    border: 1px solid var(--border-color, #e2e8f0);
                    background: var(--bg-secondary, #f8fafc);
                    color: var(--text-primary, #1e293b);
                    cursor: pointer;
                    font-size: 13px;
                    transition: all .2s;
                }
                .btn:hover {
                    background: var(--hover-bg, #e2e8f0);
                }
                .btn-primary {
                    background: var(--accent, #3b82f6);
                    color: #fff;
                    border-color: var(--accent, #3b82f6);
                }
                .btn-primary:hover {
                    background: var(--accent-dark, #2563eb);
                    border-color: var(--accent-dark, #2563eb);
                }
                .btn-icon {
                    background: none;
                    border: none;
                    cursor: pointer;
                    padding: 4px;
                    color: var(--text-secondary, #64748b);
                    border-radius: 4px;
                    transition: background 0.2s, color 0.2s;
                }
                .btn-icon:hover {
                    background: var(--hover-bg, #e2e8f0);
                    color: var(--accent, #3b82f6);
                }
                .card h3 {
                    margin: 0 0 12px 0;
                }
                /* 提供商表格列 */
                .provider-name-cell { min-width: 120px; }
                .provider-type-cell { width: 100px; }
                .provider-rule-count-cell { width: 80px; text-align: center; }
                .provider-time-cell { min-width: 160px; }
                .provider-action-cell { width: 60px; text-align: center; }
            </style>
            <div class="card">
                <h3 class="rules-title">${t('rules.title')}</h3>
                <div class="rules-toolbar">
                    <div class="view-tabs">
                        <button class="view-tab active" data-view="rules">${t('rules.rules_tab') || '规则'}</button>
                        <button class="view-tab" data-view="providers">${t('rules.providers_tab') || '规则提供者'}</button>
                    </div>
                    <input type="text" id="rule-filter" class="search-box" placeholder="${t('rules.search_placeholder')}" value="${escapeHtml(filterText)}">
                    <button id="update-all-rules" class="btn btn-primary">${t('rules.update_all')}</button>
                </div>
                <div id="rules-view" style="display:block;">
                    <div class="rules-table-wrapper">
                        <table class="rules-table">
                            <thead><tr>
                                <th class="rule-enabled">${t('rules.enabled')}</th>
                                <th class="rule-payload">${t('rules.payload')}</th>
                                <th class="rule-proxy">${t('rules.proxy')}</th>
                            </tr></thead>
                            <tbody id="rules-tbody"></tbody>
                        </table>
                        <div class="rules-footer">${t('rules.total')} <strong id="rules-count">0</strong> ${t('rules.rules_count')}</div>
                    </div>
                </div>
                <div id="providers-view" style="display:none;">
                    <div class="rules-table-wrapper">
                        <table class="rules-table">
                            <thead><tr>
                                <th class="provider-name-cell">${t('rules.provider_name') || '名称'}</th>
                                <th class="provider-type-cell">${t('rules.provider_type') || '类型'}</th>
                                <th class="provider-rule-count-cell">${t('rules.rule_count') || '规则数'}</th>
                                <th class="provider-time-cell">${t('rules.updated_at') || '更新时间'}</th>
                                <th class="provider-action-cell">${t('rules.action')}</th>
                            </tr></thead>
                            <tbody id="providers-tbody"></tbody>
                        </table>
                        <div class="rules-footer">${t('rules.total')} <strong id="providers-count">0</strong> ${t('rules.providers_count') || '个提供者'}</div>
                    </div>
                </div>
            </div>
        `;

        // 绑定视图切换
        document.querySelectorAll('.view-tab').forEach(tab => {
            tab.addEventListener('click', function() {
                const view = this.dataset.view;
                switchView(view);
            });
        });

        // 绑定搜索过滤（仅规则视图）
        const filterInput = document.getElementById('rule-filter');
        if (filterInput) {
            filterInput.addEventListener('input', e => {
                filterText = e.target.value.trim();
                if (currentView === 'rules') renderRules();
            });
        }

        // 全部更新按钮
        const updateAllBtn = document.getElementById('update-all-rules');
        if (updateAllBtn) {
            updateAllBtn.addEventListener('click', function() {
                if (currentView === 'rules') {
                    updateAllProviders().then(() => {
                        fetchRules();
                    });
                } else {
                    updateAllProviders();
                }
            });
        }

        // 初始加载
        fetchProviders();
        fetchRules();
        initLanguageListener();
    }

    async function init() {
        container = document.getElementById('rules-content');
        if (!container) return;
        render();
    }

    function destroy() {
        allRules = [];
        allProviders = [];
        if (container) container.innerHTML = '';
        if (langEventListener) {
            window.removeEventListener('languageChanged', langEventListener);
            langEventListener = null;
        }
    }

    return { init, destroy };
})();