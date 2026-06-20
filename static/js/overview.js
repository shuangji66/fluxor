// 概览模块：流量监控 + 外置面板入口 + 当前节点 + 内核状态显示（无控制）
window.Overview = (function() {
    const BASE = window.BASE_URL || '';
    let uploadSpeeds = [], downloadSpeeds = [];
    let totalUpload = 0, totalDownload = 0;
    const maxPoints = 65; // 改为 65
    let canvas, ctx;
    let themeObserver = null, resizeObserver = null;
    let dpr = window.devicePixelRatio || 1;
    let rafPending = false;
    let cachedMaxY = 1024;
    let langEventListener = null;
    let coreRunning = false;

    // WebSocket 实例
    let wsTraffic = null;
    let wsConnections = null;
    let wsMemory = null;

    // 内核状态轮询定时器
    let statusTimer = null;

    // 外置面板配置
    let uiPanel = 'metacubexd'; // 默认

    // ---------- 工具函数 ----------
    function formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(Math.abs(bytes) || 1) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/[&<>]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[m]));
    }

    function buildApiUrl(path) {
        if (!BASE || BASE === '/') return path;
        return BASE.replace(/\/$/, '') + '/' + path.replace(/^\//, '');
    }

    function t(key) {
        return (window.i18n && window.i18n.t) ? window.i18n.t(key) : key;
    }

    // ---------- 流量图表 ----------
    function getChartColors() {
        const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
        return {
            grid: isDark ? 'rgba(148,163,184,0.15)' : 'rgba(15,23,42,0.08)',
            upload: '#3b82f6', download: '#10b981',
            uploadFill: isDark ? 'rgba(59,130,246,0.1)' : 'rgba(59,130,246,0.08)',
            downloadFill: isDark ? 'rgba(16,185,129,0.1)' : 'rgba(16,185,129,0.08)',
            text: isDark ? '#94a3b8' : '#64748b'
        };
    }

    function drawChart() {
        rafPending = false;
        if (!ctx || !canvas) return;
        const w = canvas.width / dpr, h = canvas.height / dpr;
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        ctx.save();
        ctx.scale(dpr, dpr);

        if (uploadSpeeds.length < 2 && downloadSpeeds.length < 2) { ctx.restore(); return; }

        let currentMax = 1024;
        for (let i = 0; i < uploadSpeeds.length; i++) if (uploadSpeeds[i] > currentMax) currentMax = uploadSpeeds[i];
        for (let i = 0; i < downloadSpeeds.length; i++) if (downloadSpeeds[i] > currentMax) currentMax = downloadSpeeds[i];
        cachedMaxY = Math.max(currentMax, cachedMaxY * 0.95);

        const stepX = w / (maxPoints - 1);
        const colors = getChartColors();
        const paddingBottom = 18; // 底部留白
        const chartH = h - paddingBottom;

        const length = uploadSpeeds.length;
        const offsetX = (maxPoints - length) * stepX;

        // 绘制网格和刻度（y轴）
        ctx.strokeStyle = colors.grid; ctx.lineWidth = 1;
        ctx.font = '10px monospace'; ctx.textAlign = 'right'; ctx.fillStyle = colors.text;
        for (let i = 0; i <= 4; i++) {
            const y = (i / 4) * chartH;
            ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
            ctx.fillText(formatBytes(cachedMaxY * (1 - i / 4)), w - 4, y - 3);
        }

        // 绘制面积图
        function drawArea(data, stroke, fill) {
            if (data.length < 2) return;
            ctx.beginPath();
            for (let i = 0; i < data.length; i++) {
                const x = offsetX + i * stepX;
                const y = chartH - (data[i] / cachedMaxY) * chartH;
                i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
            }
            ctx.strokeStyle = stroke; ctx.lineWidth = 2; ctx.stroke();
            const lastX = offsetX + (data.length - 1) * stepX;
            ctx.lineTo(lastX, chartH);
            ctx.lineTo(offsetX, chartH);
            ctx.closePath(); ctx.fillStyle = fill; ctx.fill();
        }

        drawArea(uploadSpeeds, colors.upload, colors.uploadFill);
        drawArea(downloadSpeeds, colors.download, colors.downloadFill);

        // 图例
        ctx.font = '12px sans-serif'; ctx.textAlign = 'left';
        ctx.fillStyle = colors.upload; ctx.fillRect(10, 10, 12, 12);
        const uploadText = t('overview.upload');
        const downloadText = t('overview.download');
        ctx.fillText(uploadText, 28, 21);
        ctx.fillStyle = colors.download; ctx.fillRect(10, 28, 12, 12);
        ctx.fillText(downloadText, 28, 39);

        // ===== X 轴时间标签（与数据点位置对齐） =====
        ctx.font = '10px monospace';
        ctx.textAlign = 'center';
        ctx.fillStyle = colors.text;
        const timeLabels = [60, 45, 30, 15, 0];
        const lastIdx = length - 1;
        timeLabels.forEach(sec => {
            const idx = lastIdx - sec;
            if (idx >= 0 && idx < length) {
                const x = offsetX + idx * stepX;
                ctx.fillText(sec + 's', x, chartH + 12);
            }
        });

        ctx.restore();
    }

    function scheduleDraw() {
        if (!rafPending) { rafPending = true; requestAnimationFrame(drawChart); }
    }

    // ---------- 数据更新函数 ----------
    function updateSpeed(up, down) {
        uploadSpeeds.push(up); downloadSpeeds.push(down);
        if (uploadSpeeds.length > maxPoints) uploadSpeeds.shift();
        if (downloadSpeeds.length > maxPoints) downloadSpeeds.shift();
        const elUs = document.getElementById('ov-upload-speed');
        const elDs = document.getElementById('ov-download-speed');
        if (elUs) elUs.textContent = formatBytes(up) + '/s';
        if (elDs) elDs.textContent = formatBytes(down) + '/s';
        scheduleDraw();
    }

    function updateTotals(up, down) {
        totalUpload = up; totalDownload = down;
        const elUt = document.getElementById('ov-upload-total');
        const elDt = document.getElementById('ov-download-total');
        if (elUt) elUt.textContent = formatBytes(totalUpload);
        if (elDt) elDt.textContent = formatBytes(totalDownload);
    }

    // ---------- 更新当前节点（修改：递归解析代理组） ----------
    function updateProxySelection(proxies) {
        const container = document.getElementById('current-proxy-container');
        if (!container) return;

        const entries = Object.entries(proxies || {});
        // 查找主节点选择组（Selector 且名称包含“节点选择”）
        const mainGroup = entries.find(([, g]) => g.type === 'Selector' && g.name && g.name.includes('节点选择'));
        if (!mainGroup) {
            container.innerHTML = '<div style="color:var(--text-secondary);font-size:0.9em;">暂无节点选择</div>';
            return;
        }

        let selected = mainGroup[1].now || '-';
        // 递归解析：如果选中的名称匹配某个代理组（Selector 或 URLTest），则继续解析
        let maxLoop = 10;
        while (maxLoop-- > 0) {
            const found = entries.find(([name, g]) => name === selected && (g.type === 'Selector' || g.type === 'URLTest'));
            if (found) {
                selected = found[1].now || '-';
            } else {
                break;
            }
        }

        container.innerHTML = `<div style="font-weight:500;color:var(--accent);">${escapeHtml(selected)}</div>`;
    }

    // ---------- 获取订阅配置（面板选择） ----------
    async function fetchSubscribeConfig() {
        try {
            const resp = await fetch(buildApiUrl('/subscribe/config'));
            if (!resp.ok) return;
            const cfg = await resp.json();
            uiPanel = cfg.ui_panel || 'metacubexd';
            updatePanelLink();
        } catch (e) {
            console.warn('[Overview] 获取订阅配置失败，使用默认面板', e);
        }
    }

    function updatePanelLink() {
        const container = document.getElementById('external-control-card');
        if (!container) return;
        const panelName = uiPanel === 'zashboard' ? 'Zashboard' : 'MetaCubeXD';
        const path = uiPanel === 'zashboard' ? '/zash/' : '/meta/';
        const href = BASE + path;
        container.innerHTML = `
            <a href="${href}" target="_blank" style="display:block;text-decoration:none;color:var(--text-primary);">
                <div style="display:flex;justify-content:space-between;align-items:center;">
                    <span style="font-size:0.85em;color:var(--text-secondary);">${t('overview.external_control')}</span>
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>
                        <polyline points="15 3 21 3 21 9"/>
                        <line x1="10" y1="14" x2="21" y2="3"/>
                    </svg>
                </div>
                <div style="font-weight:500;color:var(--accent);margin-top:4px;">${panelName}</div>
            </a>
        `;
    }

    // ---------- WebSocket 建立 ----------
    function startWebSockets() {
        if (wsTraffic) wsTraffic.close();
        if (wsConnections) wsConnections.close();
        if (wsMemory) wsMemory.close();

        wsTraffic = window.API.wsConnect('/traffic', (e) => {
            try {
                let up = 0, down = 0;
                if (typeof e.data === 'string') {
                    const d = JSON.parse(e.data);
                    up = d.up || d.upload || 0;
                    down = d.down || d.download || 0;
                } else if (e.data instanceof ArrayBuffer) {
                    const v = new DataView(e.data);
                    up = Number(v.getBigUint64(0, false));
                    down = Number(v.getBigUint64(8, false));
                }
                updateSpeed(up, down);
            } catch(err) {
                console.warn('[Overview] Traffic WS error', err);
            }
        }, {
            onError: (e) => console.error('[Overview] Traffic WS error', e),
            onClose: () => console.log('[Overview] Traffic WS closed')
        });

        wsConnections = window.API.wsConnect('/connections', (e) => {
            try {
                const d = JSON.parse(e.data);
                const elConn = document.getElementById('ov-connections');
                if (elConn) elConn.textContent = d.connections ? d.connections.length : 0;
                if (d.uploadTotal !== undefined && d.downloadTotal !== undefined) {
                    updateTotals(d.uploadTotal, d.downloadTotal);
                }
            } catch(err) {
                console.warn('[Overview] Connections WS error', err);
            }
        }, {
            onError: (e) => console.error('[Overview] Connections WS error', e),
            onClose: () => console.log('[Overview] Connections WS closed')
        });

        wsMemory = window.API.wsConnect('/memory', (e) => {
            try {
                let mem = 0;
                if (typeof e.data === 'string') {
                    const d = JSON.parse(e.data);
                    mem = d.inuse || d.memory || 0;
                } else if (e.data instanceof ArrayBuffer) {
                    mem = Number(new DataView(e.data).getBigUint64(0, false));
                }
                const el = document.getElementById('ov-memory');
                if (el) el.textContent = formatBytes(mem);
            } catch(err) {
                console.warn('[Overview] Memory WS error', err);
            }
        }, {
            onError: (e) => console.error('[Overview] Memory WS error', e),
            onClose: () => console.log('[Overview] Memory WS closed')
        });
    }

    // ---------- 获取版本、内核状态和代理选择 ----------
    async function fetchVersionStatus() {
        try {
            const [versionResp, statusResp, proxiesResp] = await Promise.all([
                fetch(buildApiUrl('/version')).catch(() => null),
                fetch(buildApiUrl('/core/status')).catch(() => null),
                fetch(buildApiUrl('/proxies')).catch(() => null)
            ]);

            if (versionResp && versionResp.ok) {
                try {
                    const v = await versionResp.json();
                    const el = document.getElementById('ov-core-version');
                    let version = v.version || '';
                    version = version.replace(/^v/, '');
                    if (el) el.textContent = version;
                } catch (e) { /* ignore */ }
            } else {
                const el = document.getElementById('ov-core-version');
                if (el) el.textContent = '未知';
            }

            if (statusResp && statusResp.ok) {
                try {
                    const s = await statusResp.json();
                    coreRunning = s.running;
                } catch (e) { /* ignore */ }
            }

            if (proxiesResp && proxiesResp.ok) {
                try {
                    const data = await proxiesResp.json();
                    updateProxySelection(data.proxies);
                } catch (e) { console.warn('[Overview] 解析代理数据失败', e); }
            }
        } catch (e) {
            console.warn('[Overview] fetchVersionStatus error', e);
        }
    }

    function startStatusPolling() {
        if (statusTimer) clearInterval(statusTimer);
        fetchVersionStatus();
        statusTimer = setInterval(fetchVersionStatus, 10000);
    }

    function stopStatusPolling() {
        if (statusTimer) {
            clearInterval(statusTimer);
            statusTimer = null;
        }
    }

    // ---------- 画布初始化 ----------
    function initCanvas() {
        canvas = document.getElementById('traffic-canvas');
        if (!canvas) return;
        ctx = canvas.getContext('2d');
        dpr = window.devicePixelRatio || 1;
        const resize = () => {
            const parent = canvas.parentElement;
            if (!parent) return;
            const w = parent.clientWidth;
            const h = 260;
            canvas.style.width = w + 'px';
            canvas.style.height = h + 'px';
            canvas.width = w * dpr;
            canvas.height = h * dpr;
            scheduleDraw();
        };
        if (resizeObserver) resizeObserver.disconnect();
        resizeObserver = new ResizeObserver(resize);
        resizeObserver.observe(canvas.parentElement);
        resize();
    }

    function observeTheme() {
        if (themeObserver) themeObserver.disconnect();
        themeObserver = new MutationObserver(scheduleDraw);
        themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
    }

    // ---------- 渲染主界面（调整顺序） ----------
    function render() {
        const container = document.getElementById('overview-content');
        if (!container) return;

        container.innerHTML = `
            <div class="stats-grid">
                <!-- 上传速度 -->
                <div class="stat-box">
                    <div class="stat-label">${t('overview.upload_speed')}</div>
                    <div class="stat-value" id="ov-upload-speed">0 B/s</div>
                </div>
                <!-- 下载速度 -->
                <div class="stat-box">
                    <div class="stat-label">${t('overview.download_speed')}</div>
                    <div class="stat-value" id="ov-download-speed">0 B/s</div>
                </div>
                <!-- 上传总量 -->
                <div class="stat-box">
                    <div class="stat-label">${t('overview.upload_total')}</div>
                    <div class="stat-value" id="ov-upload-total">0 B</div>
                </div>
                <!-- 下载总量 -->
                <div class="stat-box">
                    <div class="stat-label">${t('overview.download_total')}</div>
                    <div class="stat-value" id="ov-download-total">0 B</div>
                </div>
                <!-- 内存占用 -->
                <div class="stat-box">
                    <div class="stat-label">${t('overview.memory_usage')}</div>
                    <div class="stat-value" id="ov-memory">N/A</div>
                </div>
                <!-- 活跃连接 -->
                <div class="stat-box">
                    <div class="stat-label">${t('overview.active_connections')}</div>
                    <div class="stat-value" id="ov-connections">0</div>
                </div>
                <!-- 内核版本（倒数第二个） -->
                <div class="stat-box">
                    <div class="stat-label">${t('overview.core_version')}</div>
                    <div class="stat-value" id="ov-core-version">加载中...</div>
                </div>
                <!-- 外部控制（最后一个） -->
                <div class="stat-box" id="external-control-card">
                    <a href="${BASE}/meta/" target="_blank" style="display:block;text-decoration:none;color:var(--text-primary);">
                        <div style="display:flex;justify-content:space-between;align-items:center;">
                            <span style="font-size:0.85em;color:var(--text-secondary);">${t('overview.external_control')}</span>
                            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>
                                <polyline points="15 3 21 3 21 9"/>
                                <line x1="10" y1="14" x2="21" y2="3"/>
                            </svg>
                        </div>
                        <div style="font-weight:500;color:var(--accent);margin-top:4px;">MetaCubeXD</div>
                    </a>
                </div>
            </div>

            <!-- 当前节点卡片 -->
            <div class="card" style="margin-bottom:16px;">
                <div style="display:flex; flex-wrap:wrap; align-items:center; gap:8px; line-height:1.6;">
                    <h3 style="margin:0; flex-shrink:0; line-height:inherit;">${t('overview.current_node')}</h3>
                    <div id="current-proxy-container" style="min-height:2em; flex:1 1 auto; text-align:left; word-break:break-word; line-height:inherit;">加载中...</div>
                </div>
            </div>

            <div class="card">
                <h3>${t('overview.traffic_trend')}</h3>
                <canvas id="traffic-canvas"></canvas>
            </div>
        `;

        fetchSubscribeConfig();

        initCanvas();
        observeTheme();
        startWebSockets();
        startStatusPolling();
    }

    // ---------- 语言变化处理 ----------
    function onLanguageChange() {
        destroy();
        render();
    }

    function initLanguageListener() {
        if (langEventListener) {
            window.removeEventListener('languageChanged', langEventListener);
        }
        langEventListener = onLanguageChange;
        window.addEventListener('languageChanged', langEventListener);
    }

    // ---------- 初始化 ----------
    function init() {
        console.log('[Overview] 初始化模块，BASE_URL:', BASE);
        render();
        initLanguageListener();
    }

    function destroy() {
        if (wsTraffic) { wsTraffic.close(); wsTraffic = null; }
        if (wsConnections) { wsConnections.close(); wsConnections = null; }
        if (wsMemory) { wsMemory.close(); wsMemory = null; }
        stopStatusPolling();

        if (themeObserver) themeObserver.disconnect();
        if (resizeObserver) resizeObserver.disconnect();
        uploadSpeeds = []; downloadSpeeds = [];
        totalUpload = totalDownload = 0;
        rafPending = false; cachedMaxY = 1024;
        if (langEventListener) {
            window.removeEventListener('languageChanged', langEventListener);
            langEventListener = null;
        }
    }

    return { init, destroy };
})();