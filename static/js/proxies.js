// 代理模块：增量渲染 + 并发测速 + 事件委托 + 国际化
window.Proxies = (function() {
    let container = null;
    let refreshTimer = null;
    let currentProxies = {};
    let expandedState = {};      // 持久化折叠状态
    let delayCache = {};         // { proxyName: { delay: number|null, ts: number } }
    
    // 测速并发控制
    let testingSet = new Set();   // 正在测速的节点集合
    const CONCURRENCY = 10;       // 最大并发测速数
    const TEST_URL = 'http://www.gstatic.com/generate_204';
    const TIMEOUT = 5000;
    const CACHE_DURATION = 600000; // 10 分钟缓存过期

    // ==================== 工具函数 ====================
    function t(key) {
        return (window.i18n && window.i18n.t) ? window.i18n.t(key) : key;
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/[&<>"']/g, m => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        })[m]);
    }

    function getDelayClass(delay) {
        if (delay === null || delay === undefined) return 'delay-unknown';
        if (delay < 0) return 'delay-error';
        if (delay < 200) return 'delay-good';
        if (delay < 800) return 'delay-ok';
        return 'delay-bad';
    }

    function formatDelay(cacheEntry) {
        if (!cacheEntry) return t('proxies.test');
        if (cacheEntry.delay === null) return t('proxies.timeout');
        if (cacheEntry.delay < 0) return t('proxies.error');
        return `${cacheEntry.delay}ms`;
    }

    function isCacheExpired(cacheEntry) {
        if (!cacheEntry) return true;
        return Date.now() - cacheEntry.ts > CACHE_DURATION;
    }

    // ==================== 增量 DOM 更新（不重建节点） ====================
    function updateDOM() {
        if (!container) return;
        
        document.querySelectorAll('.proxy-group-card').forEach(card => {
            const groupName = card.dataset.group;
            const group = currentProxies[groupName];
            if (!group) return;

            const currentEl = card.querySelector('.current-proxy');
            if (currentEl) {
                currentEl.textContent = `${t('proxies.current')}: ${group.now || '-'}`;
            }

            const listEl = card.querySelector('.proxy-list');
            const iconEl = card.querySelector('.toggle-icon');
            const isExpanded = expandedState[groupName] === true;
            if (listEl) listEl.style.display = isExpanded ? 'grid' : 'none';
            if (iconEl) iconEl.textContent = isExpanded ? '▼' : '▶';

            card.querySelectorAll('.proxy-item').forEach(item => {
                const proxyName = item.dataset.proxy;
                const isSelected = group.now === proxyName;
                item.classList.toggle('selected', isSelected);

                const btn = item.querySelector('.delay-test-btn');
                if (btn) {
                    const isTesting = testingSet.has(proxyName);
                    const cached = delayCache[proxyName];
                    
                    if (isTesting) {
                        btn.textContent = t('proxies.testing');
                        btn.className = 'delay-test-btn delay-testing';
                        btn.disabled = true;
                    } else {
                        btn.textContent = formatDelay(cached);
                        btn.className = `delay-test-btn ${getDelayClass(cached?.delay)}`;
                        btn.disabled = false;
                    }
                }
            });
        });
    }

    // ==================== 数据获取 ====================
    async function fetchProxies(fullRender = false) {
        try {
            const resp = await window.API.apiFetch('/proxies');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            currentProxies = data.proxies || {};
            
            if (fullRender || !container.querySelector('.proxy-group-card')) {
                renderFull();
            } else {
                updateDOM();
            }
        } catch (err) {
            console.error('[Proxies] 加载代理失败:', err);
            if (fullRender && container) {
                container.innerHTML = `<div class="card error-card"><div style="padding:40px;text-align:center;color:var(--text-secondary,#64748b);">${t('proxies.load_failed')}</div></div>`;
            }
        }
    }

    // ==================== 并发测速引擎 ====================
    async function testDelay(proxyName) {
        if (testingSet.has(proxyName)) return;
        
        const cached = delayCache[proxyName];
        if (cached && !isCacheExpired(cached)) {
            return;
        }
        
        testingSet.add(proxyName);
        updateDOM();

        try {
            const url = `/proxies/${encodeURIComponent(proxyName)}/delay?url=${encodeURIComponent(TEST_URL)}&timeout=${TIMEOUT}`;
            const resp = await window.API.apiFetch(url);
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            const delay = data.delay;
            if (delay !== undefined && delay >= 0) {
                delayCache[proxyName] = { delay: delay, ts: Date.now() };
            } else {
                delayCache[proxyName] = { delay: null, ts: Date.now() };
            }
        } catch (err) {
            console.warn('[Proxies] 测速失败:', proxyName, err);
            delayCache[proxyName] = { delay: null, ts: Date.now() };
        } finally {
            testingSet.delete(proxyName);
            updateDOM();
        }
    }

    async function testGroupDelays(groupName) {
        const group = currentProxies[groupName];
        if (!group || !group.all) return;
        
        const queue = [...group.all];
        const workers = Array.from({ length: Math.min(CONCURRENCY, queue.length) }, async () => {
            while (queue.length > 0) {
                const proxy = queue.shift();
                await testDelay(proxy);
            }
        });
        await Promise.all(workers);
    }

    async function testAllDelays() {
        const groups = Object.entries(currentProxies)
            .filter(([, g]) => ['Selector', 'URLTest', 'Fallback'].includes(g.type));
        
        if (groups.length === 0) {
            showToast(t('proxies.no_groups'), 'info');
            return;
        }
        
        showToast(t('proxies.testing_all'), 'info');
        
        const allProxies = [...new Set(groups.flatMap(([, g]) => g.all || []))];
        const queue = [...allProxies];
        
        const workers = Array.from({ length: CONCURRENCY }, async () => {
            while (queue.length > 0) {
                const proxy = queue.shift();
                await testDelay(proxy);
            }
        });
        await Promise.all(workers);
        showToast(t('proxies.test_complete'), 'success');
    }

    // ==================== 节点切换（乐观更新） ====================
    async function switchProxy(groupName, proxyName) {
        if (currentProxies[groupName]) {
            currentProxies[groupName].now = proxyName;
            updateDOM();
        }
        
        try {
            await window.API.apiFetch(`/proxies/${encodeURIComponent(groupName)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: proxyName })
            });
            showToast(`${t('proxies.switched')}: ${groupName} → ${proxyName}`, 'success');
        } catch (err) {
            console.error('[Proxies] 切换失败:', err);
            await fetchProxies(false);
            showToast(t('proxies.switch_failed') + ': ' + err.message, 'error');
        }
    }

    // ==================== Toast 提示 ====================
    function showToast(msg, type = 'info') {
        let toast = document.getElementById('proxy-toast');
        if (!toast) {
            toast = document.createElement('div');
            toast.id = 'proxy-toast';
            toast.style.cssText = 'position:fixed;top:20px;right:20px;z-index:9999;padding:12px 20px;border-radius:8px;font-size:14px;color:#fff;transition:opacity 0.3s;pointer-events:none;';
            document.body.appendChild(toast);
        }
        toast.textContent = msg;
        toast.style.background = type === 'error' ? '#ef4444' : type === 'success' ? '#22c55e' : '#3b82f6';
        toast.style.opacity = '1';
        clearTimeout(toast._timer);
        toast._timer = setTimeout(() => { toast.style.opacity = '0'; }, 2500);
    }

    // ==================== 全量渲染（仅初始化时调用） ====================
    function renderFull() {
        if (!container) return;
        
        // 获取所有要显示的组（Selector, URLTest, Fallback），并按优先级排序
        const groups = Object.entries(currentProxies)
            .filter(([, g]) => ['Selector', 'URLTest', 'Fallback'].includes(g.type))
            .sort((a, b) => {
                const aName = a[0];
                const bName = b[0];
                const getPriority = (name) => {
                    if (name.includes('节点选择')) return 0;
                    if (name.includes('手动选择')) return 1;
                    if (name.includes('自动选择')) return 2;
                    return 3;
                };
                const aPriority = getPriority(aName);
                const bPriority = getPriority(bName);
                if (aPriority !== bPriority) return aPriority - bPriority;
                return aName.localeCompare(bName);
            });

        if (groups.length === 0) {
            container.innerHTML = `<div class="card"><div style="padding:40px;text-align:center;color:var(--text-secondary,#64748b);">${t('proxies.empty')}</div></div>`;
            return;
        }

        container.innerHTML = `
            <style>
                .proxy-toolbar{display:flex;justify-content:flex-end;margin-bottom:16px;gap:8px;flex-wrap:wrap}
                .proxy-group-card{background:var(--card-bg,#fff);border:1px solid var(--border-color,#e2e8f0);border-radius:10px;margin-bottom:12px;overflow:hidden;transition:all .2s}
                .proxy-group-card:hover{border-color:var(--accent,#3b82f6);box-shadow:0 1px 3px rgba(59,130,246,0.1)}
                .group-header{display:flex;align-items:center;gap:10px;padding:14px 16px;cursor:pointer;user-select:none;background:var(--bg-secondary,#f8fafc);transition:background .15s}
                .group-header:hover{background:var(--hover-bg,#f1f5f9)}
                .toggle-icon{font-size:10px;color:var(--text-secondary,#64748b);width:14px;text-align:center;transition:transform .2s}
                .group-name{font-weight:600;color:var(--text-primary,#1e293b);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
                .group-type{font-size:11px;padding:2px 8px;border-radius:4px;background:var(--bg-primary,#e2e8f0);color:var(--text-secondary,#64748b);font-weight:600;flex-shrink:0}
                .current-proxy{font-size:13px;color:var(--accent,#3b82f6);max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex-shrink:0}
                .proxy-list{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:8px;padding:12px 16px}
                .proxy-item{display:flex;align-items:center;justify-content:space-between;padding:10px 12px;border-radius:8px;border:1px solid var(--border-color,#e2e8f0);cursor:pointer;transition:all .15s;gap:8px;background:var(--bg-primary,#f8fafc)}
                .proxy-item:hover{border-color:var(--accent,#3b82f6);background:rgba(59,130,246,0.04)}
                .proxy-item.selected{border-color:var(--accent,#3b82f6);background:rgba(59,130,246,0.08);box-shadow:inset 0 0 0 1px rgba(59,130,246,0.1)}
                .proxy-name{font-size:13px;color:var(--text-primary,#1e293b);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1;font-weight:500}
                .delay-test-btn{padding:3px 10px;border:none;border-radius:4px;font-size:11px;font-weight:600;cursor:pointer;min-width:56px;text-align:center;transition:all .15s;flex-shrink:0}
                .delay-good{background:#dcfce7;color:#16a34a;border:1px solid #bbf7d0}
                .delay-ok{background:#fef9c3;color:#ca8a04;border:1px solid #feef08}
                .delay-bad{background:#fee2e2;color:#dc2626;border:1px solid #fecaca}
                .delay-error{background:#fee2e2;color:#dc2626;border:1px solid #fecaca}
                .delay-unknown{background:var(--bg-secondary,#f1f5f9);color:var(--text-secondary,#94a3b8);border:1px solid var(--border-color,#e2e8f0)}
                .delay-testing{background:var(--bg-secondary,#f1f5f9);color:var(--accent,#3b82f6);border:1px solid var(--accent,#3b82f6);animation:pulse 1s infinite}
                @keyframes pulse{0%,100%{opacity:1}50%{opacity:.5}}
                .btn-sm{padding:6px 14px;border:1px solid var(--border-color,#e2e8f0);border-radius:6px;background:var(--bg-secondary,#f8fafc);color:var(--text-primary,#1e293b);cursor:pointer;font-size:12px;font-weight:600;transition:all .2s;display:inline-flex;align-items:center;gap:4px}
                .btn-sm:hover{background:var(--accent,#3b82f6);color:#fff;border-color:var(--accent,#3b82f6);transform:translateY(-1px);box-shadow:0 2px 8px rgba(59,130,246,0.2)}
                .btn-sm:disabled{opacity:.5;cursor:not-allowed;transform:none}
            </style>
            <div class="proxy-toolbar">
                <button class="btn-sm" id="test-all-btn">${t('proxies.test_all')}</button>
            </div>
            ${groups.map(([name, group]) => {
                if (expandedState[name] === undefined) expandedState[name] = false;
                const isExpanded = expandedState[name];
                return `
                    <div class="proxy-group-card" data-group="${escapeHtml(name)}">
                        <div class="group-header" data-group="${escapeHtml(name)}">
                            <span class="toggle-icon">${isExpanded ? '▼' : '▶'}</span>
                            <span class="group-name" title="${escapeHtml(name)}">${escapeHtml(name)}</span>
                            <span class="group-type">${escapeHtml(group.type)}</span>
                            <span class="current-proxy">${t('proxies.current')}: ${escapeHtml(group.now || '-')}</span>
                        </div>
                        <div class="proxy-list" style="display: ${isExpanded ? 'grid' : 'none'}">
                            ${(group.all || []).map(p => {
                                const cached = delayCache[p];
                                const isSelected = group.now === p;
                                return `
                                    <div class="proxy-item ${isSelected ? 'selected' : ''}" data-proxy="${escapeHtml(p)}" data-group="${escapeHtml(name)}">
                                        <span class="proxy-name" title="${escapeHtml(p)}">${escapeHtml(p)}</span>
                                        <button class="delay-test-btn ${getDelayClass(cached?.delay)}" data-proxy="${escapeHtml(p)}">${formatDelay(cached)}</button>
                                    </div>
                                `;
                            }).join('')}
                        </div>
                    </div>
                `;
            }).join('')}
        `;

        container.removeEventListener('click', handleContainerClick);
        container.addEventListener('click', handleContainerClick);
        document.getElementById('test-all-btn')?.addEventListener('click', () => testAllDelays());
    }

    // ==================== 统一事件委托 ====================
    function handleContainerClick(e) {
        const testBtn = e.target.closest('.delay-test-btn');
        if (testBtn) {
            e.stopPropagation();
            testDelay(testBtn.dataset.proxy);
            return;
        }
        const header = e.target.closest('.group-header');
        if (header) {
            const groupName = header.dataset.group;
            if (groupName) {
                expandedState[groupName] = !expandedState[groupName];
                updateDOM();
            }
            return;
        }
        const item = e.target.closest('.proxy-item');
        if (item) {
            switchProxy(item.dataset.group, item.dataset.proxy);
        }
    }

    // ==================== 生命周期 ====================
    async function init() {
        container = document.getElementById('proxies-content');
        if (!container) return;
        console.log('[Proxies] 初始化模块');
        await fetchProxies(true);
        
        if (refreshTimer) clearInterval(refreshTimer);
        refreshTimer = setInterval(() => fetchProxies(false), 10000);
    }

    function destroy() {
        if (refreshTimer) clearInterval(refreshTimer);
        if (container) container.removeEventListener('click', handleContainerClick);
        console.log('[Proxies] 销毁模块');
    }

    return { init, destroy, testAllDelays };
})();