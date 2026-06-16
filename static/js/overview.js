// 概览模块：流量监控 + 面板入口 + 内核控制 + 快捷操作
window.Overview = (function() {
    const BASE = window.BASE_URL || '';
    let uploadSpeeds = [], downloadSpeeds = [];
    let totalUpload = 0, totalDownload = 0;
    const maxPoints = 60;
    let canvas, ctx;
    let pollTimer = null;
    let themeObserver = null, resizeObserver = null;
    let dpr = window.devicePixelRatio || 1;
    let rafPending = false;
    let cachedMaxY = 1024;
    let langEventListener = null;
    let coreRunning = false;

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
        const chartH = h * 0.9;

        ctx.strokeStyle = colors.grid; ctx.lineWidth = 1;
        ctx.font = '10px monospace'; ctx.textAlign = 'right'; ctx.fillStyle = colors.text;
        for (let i = 0; i <= 4; i++) {
            const y = (i / 4) * h;
            ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
            ctx.fillText(formatBytes(cachedMaxY * (1 - i / 4)), w - 4, y - 3);
        }

        function drawArea(data, stroke, fill) {
            if (data.length < 2) return;
            ctx.beginPath();
            for (let i = 0; i < data.length; i++) {
                const x = i * stepX, y = h - (data[i] / cachedMaxY) * chartH;
                i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
            }
            ctx.strokeStyle = stroke; ctx.lineWidth = 2; ctx.stroke();
            ctx.lineTo((data.length - 1) * stepX, h); ctx.lineTo(0, h);
            ctx.closePath(); ctx.fillStyle = fill; ctx.fill();
        }

        drawArea(uploadSpeeds, colors.upload, colors.uploadFill);
        drawArea(downloadSpeeds, colors.download, colors.downloadFill);

        ctx.font = '12px sans-serif'; ctx.textAlign = 'left';
        ctx.fillStyle = colors.upload; ctx.fillRect(10, 10, 12, 12);
        const uploadText = (window.i18n && window.i18n.t('overview.upload')) || '上传';
        const downloadText = (window.i18n && window.i18n.t('overview.download')) || '下载';
        ctx.fillText(uploadText, 28, 21);
        ctx.fillStyle = colors.download; ctx.fillRect(10, 28, 12, 12);
        ctx.fillText(downloadText, 28, 39);
        ctx.restore();
    }

    function scheduleDraw() {
        if (!rafPending) { rafPending = true; requestAnimationFrame(drawChart); }
    }

    // ---------- 数据获取（轮询） ----------
    async function fetchStats() {
        try {
            const [trafficResp, memResp, connResp, versionResp, statusResp] = await Promise.all([
                fetch(BASE + '/traffic'),
                fetch(BASE + '/memory'),
                fetch(BASE + '/connections'),
                fetch(BASE + '/version'),
                fetch(BASE + '/core/status')
            ]);

            if (trafficResp.ok) {
                const t = await trafficResp.json();
                const up = t.up || t.upload || 0;
                const down = t.down || t.download || 0;
                updateSpeed(up, down);
            }

            if (memResp.ok) {
                const m = await memResp.json();
                const el = document.getElementById('ov-memory');
                if (el) el.textContent = formatBytes(m.inuse || m.memory || 0);
            }

            if (connResp.ok) {
                const c = await connResp.json();
                const elConn = document.getElementById('ov-connections');
                if (elConn) elConn.textContent = c.connections ? c.connections.length : 0;
                if (c.uploadTotal !== undefined && c.downloadTotal !== undefined) {
                    updateTotals(c.uploadTotal, c.downloadTotal);
                }
            }

            if (versionResp.ok) {
                const v = await versionResp.json();
                const el = document.getElementById('ov-core-version');
                if (el) el.textContent = `${v.type || 'Mihomo'} ${v.version || ''}`;
            } else {
                const el = document.getElementById('ov-core-version');
                if (el) el.textContent = '未知';
            }

            if (statusResp.ok) {
                const s = await statusResp.json();
                coreRunning = s.running;
                updateCoreButton();
            }
        } catch (e) {
            console.error('概览数据获取失败:', e);
        }
    }

    function startPolling() {
        if (pollTimer) clearInterval(pollTimer);
        fetchStats();
        pollTimer = setInterval(fetchStats, 3000);
    }

    function stopPolling() {
        if (pollTimer) {
            clearInterval(pollTimer);
            pollTimer = null;
        }
    }

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

    function updateCoreButton() {
        const btn = document.getElementById('ov-core-start-stop');
        if (btn) {
            if (coreRunning) {
                btn.textContent = '⏹️ 停止内核';
                btn.className = 'btn btn-danger';
            } else {
                btn.textContent = '▶️ 启动内核';
                btn.className = 'btn btn-primary';
            }
        }
    }

    // ---------- 操作事件 ----------
    async function toggleCore() {
        const url = coreRunning ? BASE + '/core/stop' : BASE + '/core/start';
        try {
            const resp = await fetch(url, { method: 'POST' });
            const result = await resp.json();
            if (resp.ok && result.status === 'ok') {
                alert(result.message || '操作成功');
                setTimeout(fetchStats, 1500);
            } else {
                alert('操作失败: ' + (result.message || result.error || ''));
            }
        } catch (e) {
            alert('网络错误: ' + e.message);
        }
    }

    async function restartCore() {
        if (!confirm('确定要重启内核吗？所有连接将断开。')) return;
        try {
            const resp = await fetch(BASE + '/core/restart', { method: 'POST' });
            const result = await resp.json();
            if (resp.ok && result.status === 'ok') {
                alert('重启指令已发送');
                setTimeout(fetchStats, 2000);
            } else {
                alert('重启失败: ' + (result.message || result.error || ''));
            }
        } catch (e) {
            alert('网络错误: ' + e.message);
        }
    }

    async function updateMeta() {
        const btn = document.getElementById('ov-update-meta');
        if (!btn) return;
        btn.disabled = true;
        btn.textContent = '⏳';
        try {
            const resp = await fetch(BASE + '/update/meta', { method: 'POST' });
            const data = await resp.json();
            alert(data.message || '更新操作完成');
        } catch (e) {
            alert('更新失败: ' + e.message);
        } finally {
            btn.disabled = false;
            btn.textContent = '🔄';
        }
    }

    async function updateZash() {
        const btn = document.getElementById('ov-update-zash');
        if (!btn) return;
        btn.disabled = true;
        btn.textContent = '⏳';
        try {
            const resp = await fetch(BASE + '/update/zash', { method: 'POST' });
            const data = await resp.json();
            alert(data.message || '更新操作完成');
        } catch (e) {
            alert('更新失败: ' + e.message);
        } finally {
            btn.disabled = false;
            btn.textContent = '🔄';
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

    // ---------- 渲染主界面 ----------
    function render() {
        const container = document.getElementById('overview-content');
        if (!container) return;
        const t = (window.i18n && window.i18n.t) || (key => key);

        container.innerHTML = `
            <div class="panel-cards">
                <div class="panel-card meta-card">
                    <a class="panel-link" href="${escapeHtml(BASE)}/meta/" target="_blank">MetaCubeXD</a>
                    <button class="panel-update-btn" id="ov-update-meta" title="更新 MetaCubeXD">🔄</button>
                </div>
                <div class="panel-card zash-card">
                    <a class="panel-link" href="${escapeHtml(BASE)}/zash/" target="_blank">Zashboard</a>
                    <button class="panel-update-btn" id="ov-update-zash" title="更新 Zashboard">🔄</button>
                </div>
            </div>

            <div class="stats-grid">
                <div class="stat-box"><div class="stat-label">${t('overview.core_version')}</div><div class="stat-value" id="ov-core-version">加载中...</div></div>
                <div class="stat-box"><div class="stat-label">${t('overview.upload_speed')}</div><div class="stat-value" id="ov-upload-speed">0 B/s</div></div>
                <div class="stat-box"><div class="stat-label">${t('overview.download_speed')}</div><div class="stat-value" id="ov-download-speed">0 B/s</div></div>
                <div class="stat-box"><div class="stat-label">${t('overview.upload_total')}</div><div class="stat-value" id="ov-upload-total">0 B</div></div>
                <div class="stat-box"><div class="stat-label">${t('overview.download_total')}</div><div class="stat-value" id="ov-download-total">0 B</div></div>
                <div class="stat-box"><div class="stat-label">${t('overview.memory_usage')}</div><div class="stat-value" id="ov-memory">N/A</div></div>
                <div class="stat-box"><div class="stat-label">${t('overview.active_connections')}</div><div class="stat-value" id="ov-connections">0</div></div>
            </div>

            <div class="card">
                <h3>${t('overview.traffic_trend')}</h3>
                <canvas id="traffic-canvas"></canvas>
            </div>

            <div class="button-group">
                <button id="ov-core-start-stop" class="btn">⏳ 检测中...</button>
                <button class="btn" id="ov-core-restart">🔄 重启内核</button>
            </div>
        `;

        // 绑定事件
        document.getElementById('ov-core-start-stop').onclick = toggleCore;
        document.getElementById('ov-core-restart').onclick = restartCore;
        document.getElementById('ov-update-meta').onclick = updateMeta;
        document.getElementById('ov-update-zash').onclick = updateZash;

        initCanvas();
        observeTheme();
        startPolling();
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
        render();
        initLanguageListener();
    }

    function destroy() {
        stopPolling();
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