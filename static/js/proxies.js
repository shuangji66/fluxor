// 代理模块：双列独立布局 + 组/节点健康度 + 并发测速 + 事件委托 + 国际化
window.Proxies = (function() {
    let container = null;
    let refreshTimer = null;
    let currentProxies = {};
    let expandedState = {};
    let delayCache = {};

    let testingSet = new Set();
    // 移除 autoTestTimer

    const CONCURRENCY = 10;
    const TEST_URL = 'http://www.gstatic.com/generate_204';
    const TIMEOUT = 5000;
    const CACHE_DURATION = 600000; // 10 分钟

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

    function escapeCssSelector(str) {
        if (!str) return '';
        return str.replace(/([ "':;<=>])/g, '\\$1');
    }

    // 获取组优先级（用于排序和分列）
    function getGroupPriority(name) {
        if (name.includes('节点选择')) return 0;
        if (name.includes('手动选择')) return 1;
        if (name.includes('自动选择')) return 2;
        return 3;
    }

    // 判断组是否应使用长条健康度（自动选择、手动选择、地区组、GLOBAL）
    function shouldUseBar(groupName) {
        if (!groupName) return false;
        const barKeywords = ['自动选择', '手动选择', '美国', '日本', '香港', '新加坡', '台湾'];
        return barKeywords.some(kw => groupName.includes(kw)) || groupName === 'GLOBAL';
    }

    // 测速按钮颜色类（阈值与健康度一致，0/无记录为未知）
    function getDelayClass(delay) {
        if (delay === undefined || delay === null || delay === 0) return 'delay-unknown';
        if (delay < 500) return 'delay-good';
        if (delay < 1500) return 'delay-ok';
        return 'delay-bad';
    }

    // 格式化测速显示：无记录或0显示 ---，否则显示 delayms
    function formatDelay(cacheEntry) {
        if (!cacheEntry) return '---';
        const delay = cacheEntry.delay;
        if (delay === null || delay === undefined || delay === 0) return '---';
        return `${delay}ms`;
    }

    // ==================== 健康度渲染 ====================
    // 小卡片健康度条（节点）
    function renderNodeHealthBar(history) {
        if (!history || history.length === 0) {
            // 灰色占位条
            return `<span style="flex:1;height:100%;background:var(--border-color);border-radius:2px;"></span>`;
        }
        const sorted = [...history].sort((a, b) => new Date(a.time) - new Date(b.time));
        const segments = sorted.slice(-5);
        return segments.map(seg => {
            const delay = seg.delay;
            let color = 'var(--border-color)';
            if (delay === 0) color = '#000000';
            else if (delay >= 1 && delay <= 500) color = '#22c55e';
            else if (delay >= 501 && delay <= 1500) color = '#eab308';
            else if (delay > 1500) color = '#ef4444';
            return `<span style="flex:1;height:100%;background:${color};border-radius:2px;"></span>`;
        }).join('');
    }

    // 获取节点最新延迟（返回 delay 值，若无则 null）
    function getLatestDelay(nodeData) {
        if (!nodeData || !nodeData.history || nodeData.history.length === 0) return null;
        const sorted = [...nodeData.history].sort((a, b) => new Date(a.time) - new Date(b.time));
        return sorted[sorted.length - 1].delay;
    }

    // 组健康度：圆点（普通组）
    function renderGroupDots(group, groupName) {
        const nodes = group.all || [];
        if (nodes.length === 0) return '';
        // 对每个节点取最新延迟
        return nodes.map(name => {
            const nodeData = currentProxies[name];
            const delay = getLatestDelay(nodeData);
            let color = 'var(--border-color)'; // 灰色（无记录）
            if (delay === 0) color = '#000000';
            else if (delay >= 1 && delay <= 500) color = '#22c55e';
            else if (delay >= 501 && delay <= 1500) color = '#eab308';
            else if (delay > 1500) color = '#ef4444';
            const isSelected = group.now === name;
            const dotStyle = `display:inline-block;width:10px;height:10px;border-radius:50%;background:${color};margin-right:4px;position:relative;flex-shrink:0;`;
            const innerDot = isSelected ? `<span style="position:absolute;top:2px;left:2px;width:6px;height:6px;border-radius:50%;background:white;"></span>` : '';
            return `<span style="${dotStyle}">${innerDot}</span>`;
        }).join('');
    }

    // 组健康度：长条（自动选择/手动选择/地区组/GLOBAL）
    function renderGroupBar(group) {
        const nodes = group.all || [];
        if (nodes.length === 0) return '';
        let green = 0, yellow = 0, red = 0, black = 0, none = 0;
        nodes.forEach(name => {
            const nodeData = currentProxies[name];
            const delay = getLatestDelay(nodeData);
            if (delay === null) none++;
            else if (delay === 0) black++;
            else if (delay >= 1 && delay <= 500) green++;
            else if (delay >= 501 && delay <= 1500) yellow++;
            else if (delay > 1500) red++;
        });
        const total = nodes.length;
        if (total === 0) return '';
        const greenPct = (green / total) * 100;
        const yellowPct = (yellow / total) * 100;
        const redPct = (red / total) * 100;
        const blackPct = (black / total) * 100;
        const nonePct = (none / total) * 100;

        // 构建色块（顺序：绿、黄、红、黑、灰）
        const blocks = [];
        if (greenPct > 0) blocks.push(`<span style="flex:${greenPct};background:#22c55e;height:100%;border-radius:2px 0 0 2px;"></span>`);
        if (yellowPct > 0) blocks.push(`<span style="flex:${yellowPct};background:#eab308;height:100%;"></span>`);
        if (redPct > 0) blocks.push(`<span style="flex:${redPct};background:#ef4444;height:100%;"></span>`);
        if (blackPct > 0) blocks.push(`<span style="flex:${blackPct};background:#000000;height:100%;"></span>`);
        if (nonePct > 0) blocks.push(`<span style="flex:${nonePct};background:var(--border-color);height:100%;border-radius:0 2px 2px 0;"></span>`);
        if (blocks.length === 0) return '';
        return blocks.join('');
    }

    // ==================== 组测速功能（手动，显示 Toast） ====================
    async function testGroupDelays(groupName) {
        const group = currentProxies[groupName];
        if (!group || !group.all || group.all.length === 0) {
            showToast(t('proxies.no_nodes'), 'info');
            return;
        }
        showToast(`正在测速 ${groupName}...`, 'info');
        const queue = [...group.all];
        const workers = Array.from({ length: Math.min(CONCURRENCY, queue.length) }, async () => {
            while (queue.length > 0) {
                const proxy = queue.shift();
                await testDelay(proxy);
            }
        });
        await Promise.all(workers);
        showToast(`${groupName} ${t('proxies.test_complete')}`, 'success');
    }

    // ==================== 自动测速（无 Toast，仅对无记录的节点） ====================
    async function autoTestMissingNodes() {
        // 收集所有需要测速的节点（从各组中获取）
        const allNodes = new Set();
        const groups = Object.entries(currentProxies)
            .filter(([, g]) => ['Selector', 'URLTest', 'Fallback'].includes(g.type));
        groups.forEach(([, g]) => {
            if (g.all) g.all.forEach(node => allNodes.add(node));
        });

        // 过滤出无有效延迟记录的节点
        const needTest = [];
        for (const name of allNodes) {
            const cached = delayCache[name];
            if (!cached || cached.delay === null || cached.delay === 0) {
                needTest.push(name);
            }
        }

        if (needTest.length === 0) return;

        // 并发测速，不显示 Toast
        const queue = [...needTest];
        const workers = Array.from({ length: Math.min(CONCURRENCY, queue.length) }, async () => {
            while (queue.length > 0) {
                const proxy = queue.shift();
                await testDelay(proxy);
            }
        });
        await Promise.all(workers);
    }

    // ==================== 增量 DOM 更新 ====================
    function updateDOM(groupName) {
        if (!container) return;

        const selector = groupName
            ? `.proxy-group-card[data-group="${escapeCssSelector(groupName)}"]`
            : '.proxy-group-card';

        document.querySelectorAll(selector).forEach(card => {
            const gName = card.dataset.group;
            const group = currentProxies[gName];
            if (!group) return;

            // 更新当前选择（第二行）
            const currentEl = card.querySelector('.current-proxy');
            if (currentEl) currentEl.textContent = group.now || '-';

            // 更新组健康度
            const healthIndicator = card.querySelector('.group-health');
            if (healthIndicator) {
                const useBar = shouldUseBar(gName);
                if (useBar) {
                    healthIndicator.innerHTML = renderGroupBar(group);
                } else {
                    healthIndicator.innerHTML = renderGroupDots(group, gName);
                }
            }

            // 展开/折叠
            const listEl = card.querySelector('.proxy-list');
            const isExpanded = expandedState[gName] === true;
            if (listEl) listEl.style.display = isExpanded ? 'grid' : 'none';

            // 更新节点小卡片
            card.querySelectorAll('.proxy-item').forEach(item => {
                const proxyName = item.dataset.proxy;
                const nodeData = currentProxies[proxyName];
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

                const healthBar = item.querySelector('.health-bar');
                if (healthBar) {
                    const history = nodeData && nodeData.history ? nodeData.history : [];
                    healthBar.innerHTML = renderNodeHealthBar(history);
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

            if (fullRender || !container.querySelector('.proxy-grid')) {
                renderFull();
                // 首次加载完成后，触发自动测速（无 Toast）
                if (fullRender) {
                    autoTestMissingNodes().catch(e => console.warn('[Proxies] 自动测速出错:', e));
                }
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

    // ==================== 并发测速引擎（单个节点） ====================
    async function testDelay(proxyName) {
        if (testingSet.has(proxyName)) return;

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
            await fetchProxies(false);
        }
    }

    // ==================== 全部测速（显示 Toast） ====================
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

    // ==================== 节点切换 ====================
    async function switchProxy(groupName, proxyName) {
        if (currentProxies[groupName]) {
            currentProxies[groupName].now = proxyName;
            updateDOM(groupName);
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

    // ==================== Toast ====================
    function showToast(msg, type = 'info') {
        let toast = document.getElementById('proxy-toast');
        if (!toast) {
            toast = document.createElement('div');
            toast.id = 'proxy-toast';
            toast.style.cssText = 'position:fixed;right:20px;z-index:9999;padding:12px 20px;border-radius:8px;font-size:14px;color:#fff;transition:opacity 0.3s;pointer-events:none;';
            document.body.appendChild(toast);
        }
        const isMobile = window.innerWidth <= 768;
        toast.style.top = isMobile ? '70px' : '20px';
        toast.textContent = msg;
        toast.style.background = type === 'error' ? '#ef4444' : type === 'success' ? '#22c55e' : '#3b82f6';
        toast.style.opacity = '1';
        clearTimeout(toast._timer);
        toast._timer = setTimeout(() => { toast.style.opacity = '0'; }, 2500);
    }

    // ==================== 全量渲染 ====================
    function renderFull() {
        if (!container) return;

        const groups = Object.entries(currentProxies)
            .filter(([, g]) => ['Selector', 'URLTest', 'Fallback'].includes(g.type))
            .sort((a, b) => {
                const aName = a[0];
                const bName = b[0];
                const aPriority = getGroupPriority(aName);
                const bPriority = getGroupPriority(bName);
                if (aPriority !== bPriority) return aPriority - bPriority;
                return aName.localeCompare(bName);
            });

        if (groups.length === 0) {
            container.innerHTML = `<div class="card"><div style="padding:40px;text-align:center;color:var(--text-secondary,#64748b);">${t('proxies.empty')}</div></div>`;
            return;
        }

        // 分列：重要组（节点选择、手动选择、自动选择）全部放入左列，普通组交替放入左右列
        const leftGroups = [];
        const rightGroups = [];
        let normalIdx = 0;
        groups.forEach(g => {
            const name = g[0];
            const priority = getGroupPriority(name);
            if (priority < 3) {
                leftGroups.push(g);
            } else {
                if (normalIdx % 2 === 0) leftGroups.push(g);
                else rightGroups.push(g);
                normalIdx++;
            }
        });

        function renderColumn(groupsArr) {
            return groupsArr.map(([name, group]) => {
                if (expandedState[name] === undefined) expandedState[name] = false;
                const isExpanded = expandedState[name];
                const current = group.now || '-';
                const useBar = shouldUseBar(name);
                const healthHtml = useBar ? renderGroupBar(group) : renderGroupDots(group, name);

                return `
                    <div class="proxy-group-card" data-group="${escapeHtml(name)}">
                        <!-- 第一行：组名 + 测速按钮 -->
                        <div style="display:flex;justify-content:space-between;align-items:center;">
                            <span class="group-name">${escapeHtml(name)}</span>
                            <button class="btn-group-test" data-group="${escapeHtml(name)}" title="${t('proxies.test_group')}" style="background:none;border:none;cursor:pointer;color:var(--text-secondary);padding:4px;flex-shrink:0;">
                                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                                    <polyline points="23 4 23 10 17 10"/>
                                    <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
                                </svg>
                            </button>
                        </div>
                        <!-- 第二行：当前选中节点（左对齐） -->
                        <div class="current-proxy" style="font-size:13px;color:var(--accent);margin-top:2px;text-align:left;">${escapeHtml(current)}</div>
                        <!-- 第三行：健康度 -->
                        <div class="group-health" style="margin-top:6px;display:flex;gap:4px;height:6px;align-items:center;flex-wrap:wrap;">
                            ${healthHtml}
                        </div>
                        <!-- 节点列表 -->
                        <div class="proxy-list" style="display: ${isExpanded ? 'grid' : 'none'}">
                            ${(group.all || []).map(p => {
                                const nodeData = currentProxies[p];
                                const history = nodeData && nodeData.history ? nodeData.history : [];
                                const cached = delayCache[p];
                                const isSelected = group.now === p;
                                const type = nodeData ? (nodeData.type || '') : '';
                                const udp = nodeData && nodeData.udp === true;
                                const xudp = nodeData && nodeData.xudp === true;
                                let protoTags = '';
                                if (udp) protoTags += '<span class="proto-tag">UDP</span>';
                                if (xudp) protoTags += '<span class="proto-tag">XUDP</span>';

                                return `
                                    <div class="proxy-item ${isSelected ? 'selected' : ''}" data-proxy="${escapeHtml(p)}" data-group="${escapeHtml(name)}">
                                        <div class="proxy-item-row" style="display:flex;align-items:center;width:100%;gap:6px;">
                                            <span class="proxy-name" title="${escapeHtml(p)}" style="flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:500;">${escapeHtml(p)}</span>
                                            <button class="delay-test-btn ${getDelayClass(cached?.delay)}" data-proxy="${escapeHtml(p)}" style="flex-shrink:0;">${formatDelay(cached)}</button>
                                        </div>
                                        <div class="proxy-protocol" style="display:flex;gap:6px;font-size:10px;color:var(--text-secondary);margin-top:2px;flex-wrap:wrap;">
                                            ${type ? `<span class="proto-type" style="background:var(--bg-secondary);padding:0 6px;border-radius:3px;">${escapeHtml(type)}</span>` : ''}
                                            ${protoTags}
                                        </div>
                                        <div class="health-bar" style="display:flex;gap:2px;width:100%;margin-top:4px;height:4px;">
                                            ${renderNodeHealthBar(history)}
                                        </div>
                                    </div>
                                `;
                            }).join('')}
                        </div>
                    </div>
                `;
            }).join('');
        }

        let html = `
            <style>
                .proxy-toolbar{display:flex;justify-content:flex-start;margin-bottom:16px;gap:8px;flex-wrap:wrap}
                .proxy-grid{
                    display:flex;
                    gap:16px;
                    margin-bottom:16px;
                    max-width:100%;
                }
                .proxy-column{
                    flex:1;
                    display:flex;
                    flex-direction:column;
                    gap:16px;
                    min-width:0;
                }
                .proxy-group-card{
                    background:var(--card-bg,#fff);
                    border:1px solid var(--border-color,#e2e8f0);
                    border-radius:12px;
                    padding:16px;
                    cursor:pointer;
                    transition:all .2s;
                    box-shadow:var(--shadow,0 1px 3px rgba(0,0,0,0.06));
                    box-sizing:border-box;
                }
                .proxy-group-card:hover{border-color:var(--accent,#3b82f6);box-shadow:0 4px 12px rgba(59,130,246,0.12)}
                .proxy-group-card .group-name{font-weight:600;font-size:15px;color:var(--text-primary,#1e293b)}
                .proxy-group-card .current-proxy{font-size:13px;color:var(--accent,#3b82f6);margin-top:2px;text-align:left;}
                .proxy-group-card .group-health{display:flex;gap:4px;height:6px;align-items:center;flex-wrap:wrap;margin-top:6px;}
                .proxy-group-card .proxy-list{display:none;grid-template-columns:1fr;gap:6px;margin-top:12px;border-top:1px solid var(--border-color,#e2e8f0);padding-top:12px}
                .proxy-item{
                    display:flex;
                    flex-direction:column;
                    padding:8px 10px;
                    border-radius:6px;
                    border:1px solid var(--border-color,#e2e8f0);
                    cursor:pointer;
                    transition:all .15s;
                    background:var(--bg-primary,#f8fafc);
                }
                .proxy-item:hover{border-color:var(--accent,#3b82f6);background:rgba(59,130,246,0.04)}
                .proxy-item.selected{border-color:var(--accent,#3b82f6);background:rgba(59,130,246,0.08);box-shadow:inset 0 0 0 1px rgba(59,130,246,0.1)}
                .proxy-item .proxy-name{font-size:13px;color:var(--text-primary,#1e293b)}
                .proxy-item .health-bar{display:flex;gap:2px;width:100%;margin-top:4px;height:4px;}
                .proxy-item .health-bar span{flex:1;height:100%;border-radius:2px;}
                .proxy-item .proxy-protocol .proto-tag{background:var(--bg-secondary);padding:0 6px;border-radius:3px;}
                .delay-test-btn{padding:2px 10px;border:none;border-radius:4px;font-size:11px;font-weight:600;cursor:pointer;min-width:52px;text-align:center;transition:all .15s;flex-shrink:0}
                .delay-good{background:#dcfce7;color:#16a34a;border:1px solid #bbf7d0}
                .delay-ok{background:#fef9c3;color:#ca8a04;border:1px solid #feef08}
                .delay-bad{background:#fee2e2;color:#dc2626;border:1px solid #fecaca}
                .delay-unknown{background:var(--bg-secondary,#f1f5f9);color:var(--text-secondary,#94a3b8);border:1px solid var(--border-color,#e2e8f0)}
                .delay-testing{background:var(--bg-secondary,#f1f5f9);color:var(--accent,#3b82f6);border:1px solid var(--accent,#3b82f6);animation:pulse 1s infinite}
                @keyframes pulse{0%,100%{opacity:1}50%{opacity:.5}}
                .btn-sm{padding:6px 14px;border:1px solid var(--border-color,#e2e8f0);border-radius:6px;background:var(--bg-secondary,#f8fafc);color:var(--text-primary,#1e293b);cursor:pointer;font-size:12px;font-weight:600;transition:all .2s;display:inline-flex;align-items:center;gap:4px}
                .btn-sm:hover{background:var(--accent,#3b82f6);color:#fff;border-color:var(--accent,#3b82f6);transform:translateY(-1px);box-shadow:0 2px 8px rgba(59,130,246,0.2)}
                .btn-sm:disabled{opacity:.5;cursor:not-allowed;transform:none}
                .btn-group-test{background:none;border:none;cursor:pointer;color:var(--text-secondary);padding:4px;flex-shrink:0}
                .btn-group-test:hover{color:var(--accent)}
                @media (max-width:600px){
                    .proxy-grid{flex-direction:column;gap:16px}
                    .proxy-column{flex:1;gap:12px}
                }
            </style>
            <div class="proxy-toolbar">
                <button class="btn-sm" id="test-all-btn">${t('proxies.test_all')}</button>
            </div>
            <div class="proxy-grid">
                <div class="proxy-column">${renderColumn(leftGroups)}</div>
                <div class="proxy-column">${renderColumn(rightGroups)}</div>
            </div>
        `;

        container.innerHTML = html;

        container.removeEventListener('click', handleContainerClick);
        container.addEventListener('click', handleContainerClick);
        document.getElementById('test-all-btn')?.addEventListener('click', () => testAllDelays());
    }

    // ==================== 统一事件委托 ====================
    function handleContainerClick(e) {
        // 1. 节点测速按钮
        const testBtn = e.target.closest('.delay-test-btn');
        if (testBtn) {
            e.stopPropagation();
            testDelay(testBtn.dataset.proxy);
            return;
        }

        // 2. 组测速按钮
        const groupTestBtn = e.target.closest('.btn-group-test');
        if (groupTestBtn) {
            e.stopPropagation();
            const groupName = groupTestBtn.dataset.group;
            if (groupName) testGroupDelays(groupName);
            return;
        }

        // 3. 代理项点击（切换代理）
        const item = e.target.closest('.proxy-item');
        if (item) {
            e.stopPropagation();
            switchProxy(item.dataset.group, item.dataset.proxy);
            return;
        }

        // 4. 卡片点击（切换折叠，仅更新当前卡片）
        const card = e.target.closest('.proxy-group-card');
        if (card) {
            const groupName = card.dataset.group;
            if (groupName) {
                expandedState[groupName] = !expandedState[groupName];
                updateDOM(groupName);
            }
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