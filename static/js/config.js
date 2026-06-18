// 配置模块：可视化修改核心配置，支持实时保存和操作按钮
window.Config = (function() {
    const api = window.API;

    // 端口校验（0 或 1025-65535）
    function validateConfig(payload) {
        const ports = ['mixed-port', 'port', 'socks-port', 'redir-port', 'tproxy-port'];
        for (const key of ports) {
            const val = payload[key];
            if (val !== undefined && val !== null && val !== 0 && (val < 1025 || val > 65535)) {
                console.error(`${key} 端口号必须为 0（禁用）或在 1025-65535 之间`);
                return false;
            }
        }
        const usedPorts = ports.map(k => payload[k]).filter(p => p && p !== 0);
        if (new Set(usedPorts).size !== usedPorts.length) {
            console.error('存在重复的端口配置，请检查');
            return false;
        }
        return true;
    }

    let currentConfig = null;
    let saveTimeout = null;
    let isSaving = false;
    let abortController = null;
    let container = null;

    let coreRunning = false;
    let coreStatusTimer = null;
    let _populating = false;
    let _lastCoreStatus = null; // 用于避免重复更新UI

    const t = (key) => (window.i18n && window.i18n.t) ? window.i18n.t(key) : key;

    // ---------- 获取内核配置（全量渲染） ----------
    async function fetchConfig() {
        try {
            const resp = await api.apiFetch('/configs');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            currentConfig = typeof data === 'string' ? JSON.parse(data) : data;
            renderForm();
        } catch (err) {
            console.error('获取配置失败:', err);
            currentConfig = {};
            renderForm();
        }
    }

    // ---------- 仅刷新数据（不重建DOM） ----------
    async function refreshConfigData() {
        try {
            const resp = await api.apiFetch('/configs');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            currentConfig = typeof data === 'string' ? JSON.parse(data) : data;
            // 只填充表单，不重建DOM
            populateForm();
        } catch (err) {
            console.error('刷新配置数据失败:', err);
        }
    }

    // ---------- 收集内核配置变更（仅变更字段） ----------
    function collectCoreFormValues() {
        if (!currentConfig) return null;

        const getVal = (id, type = 'string') => {
            const el = document.getElementById(id);
            if (!el) return undefined;
            if (type === 'boolean') return el.checked;
            if (type === 'number') {
                const v = parseInt(el.value);
                return isNaN(v) ? undefined : v;
            }
            if (type === 'string') {
                const v = el.value.trim();
                return v === '' ? undefined : v;
            }
            return el.value;
        };

        const payload = {};

        // ---- 顶层字段 ----
        const fields = [
            { id: 'cfg-allow-lan', key: 'allow-lan', type: 'boolean' },
            { id: 'cfg-ipv6', key: 'ipv6', type: 'boolean' },
            { id: 'cfg-mode', key: 'mode', type: 'string' },
            { id: 'cfg-interface-name', key: 'interface-name', type: 'string' },
            { id: 'cfg-mixed-port', key: 'mixed-port', type: 'number' },
            { id: 'cfg-http-port', key: 'port', type: 'number' },
            { id: 'cfg-socks-port', key: 'socks-port', type: 'number' },
            { id: 'cfg-redir-port', key: 'redir-port', type: 'number' },
            { id: 'cfg-tproxy-port', key: 'tproxy-port', type: 'number' },
        ];

        for (const f of fields) {
            const val = getVal(f.id, f.type);
            const currentVal = currentConfig[f.key];
            if (val !== undefined && val !== currentVal) {
                payload[f.key] = val;
            }
        }

        // ---- TUN 字段（仅当有变化时添加） ----
        const currentTun = currentConfig.tun || {};
        const newEnable = document.getElementById('cfg-tun-enable').checked;
        const newStack = document.getElementById('cfg-tun-stack').value;
        const newDevice = document.getElementById('cfg-tun-device').value.trim();

        const curEnable = currentTun.enable;
        const curStack = currentTun.stack || '';
        const curDevice = currentTun.device || '';

        if (newEnable !== curEnable || newStack !== curStack || newDevice !== curDevice) {
            const tunDiff = {};
            if (newEnable !== curEnable) tunDiff.enable = newEnable;
            if (newStack !== curStack) tunDiff.stack = newStack;
            if (newDevice !== curDevice) tunDiff.device = newDevice;
            payload.tun = tunDiff;
        }

        // 清理 undefined
        Object.keys(payload).forEach(k => {
            if (payload[k] === undefined) delete payload[k];
        });

        console.log('[Config] 发送的配置 payload:', JSON.stringify(payload, null, 2));
        return payload;
    }

    // ---------- 保存内核配置（直接发送，无分步） ----------
    function saveCoreDebounced() {
        if (_populating) return;
        if (saveTimeout) clearTimeout(saveTimeout);
        saveTimeout = setTimeout(saveCore, 500);
    }

    async function saveCoreImmediate() {
        if (_populating) return;
        if (saveTimeout) {
            clearTimeout(saveTimeout);
            saveTimeout = null;
        }
        await saveCore();
    }

    async function saveCore() {
        if (!currentConfig || isSaving) return;
        const payload = collectCoreFormValues();
        if (!payload || Object.keys(payload).length === 0) {
            console.log('[Config] 没有变更，不保存');
            return;
        }
        if (!validateConfig(payload)) return;

        if (abortController) abortController.abort();
        abortController = new AbortController();
        isSaving = true;

        try {
            const resp = await api.apiFetch('/configs', {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
                signal: abortController.signal
            });

            if (!resp.ok) {
                const errText = await resp.text();
                throw new Error(`保存失败: ${resp.status} ${errText}`);
            }

            // 保存成功，仅刷新数据（不重建DOM，避免闪烁）
            await refreshConfigData();
        } catch (err) {
            if (err.name !== 'AbortError') {
                console.error('保存失败:', err);
                alert('保存失败: ' + err.message);
            }
        } finally {
            isSaving = false;
            abortController = null;
        }
    }

    // ---------- 内核状态 ----------
    async function fetchCoreStatus() {
        try {
            const resp = await api.apiFetch('/core/status');
            if (!resp.ok) return;
            const data = await resp.json();
            const running = data.running === true;
            // 只有状态变化时才更新UI
            if (running !== _lastCoreStatus) {
                _lastCoreStatus = running;
                coreRunning = running;
                updateCoreUI();
            }
        } catch (e) {
            console.warn('[Config] 获取内核状态失败', e);
        }
    }

    function updateCoreUI() {
        const indicator = document.getElementById('core-status-indicator');
        const text = document.getElementById('core-status-text');
        if (indicator) {
            indicator.style.background = coreRunning ? 'var(--success, #10b981)' : 'var(--danger, #ef4444)';
        }
        if (text) {
            text.textContent = coreRunning ? t('config.core_running') : t('config.core_stopped');
        }

        const toggleBtn = document.getElementById('op-core-toggle');
        if (toggleBtn) {
            toggleBtn.textContent = coreRunning ? t('config.stop_core') : t('config.start_core');
            toggleBtn.className = coreRunning ? 'btn btn-danger' : 'btn btn-success';
            toggleBtn.disabled = false;
            toggleBtn.onclick = coreRunning ? stopCore : startCore;
        }
    }

    function startCoreStatusPolling() {
        if (coreStatusTimer) clearInterval(coreStatusTimer);
        _lastCoreStatus = null; // 重置状态
        fetchCoreStatus();
        coreStatusTimer = setInterval(fetchCoreStatus, 5000);
    }

    function stopCoreStatusPolling() {
        if (coreStatusTimer) {
            clearInterval(coreStatusTimer);
            coreStatusTimer = null;
        }
    }

    async function startCore() {
        try {
            const resp = await api.apiFetch('/core/start', { method: 'POST' });
            const result = await resp.json();
            if (resp.ok && result.status === 'ok') {
                alert(t('config.core_start_success') || '内核启动成功');
                await fetchCoreStatus();
            } else {
                alert(t('config.core_start_failed') + ': ' + (result.message || result.error || ''));
            }
        } catch (e) {
            alert(t('common.network_error') + ': ' + e.message);
        }
    }

    async function stopCore() {
        if (!confirm(t('config.confirm_stop_core') || '确定要停止内核吗？所有连接将断开。')) return;
        try {
            const resp = await api.apiFetch('/core/stop', { method: 'POST' });
            const result = await resp.json();
            if (resp.ok && result.status === 'ok') {
                alert(t('config.core_stopped_success') || '内核已停止');
                await fetchCoreStatus();
            } else {
                alert(t('config.core_stop_failed') + ': ' + (result.message || result.error || ''));
            }
        } catch (e) {
            alert(t('common.network_error') + ': ' + e.message);
        }
    }

    // ---------- 操作按钮 ----------
    async function reloadConfig() {
        try {
            const resp = await api.apiFetch('/configs', { method: 'PUT' });
            if (resp.ok) {
                await fetchConfig();
                alert(t('config.reload_success') || '配置已热重载');
            } else {
                throw new Error(t('config.reload_failed') || '重载失败');
            }
        } catch (e) {
            alert((t('config.reload_failed') || '重载失败') + ': ' + e.message);
        }
    }

    async function restartCore() {
        if (!confirm(t('config.confirm_restart') || '确定要重启内核吗？所有连接将断开。')) return;
        try {
            const resp = await api.apiFetch('/restart', { method: 'POST' });
            if (!resp.ok) throw new Error(t('config.restart_failed') || '重启失败');
            alert(t('config.restart_sent') || '重启指令已发送');
        } catch (e) {
            alert((t('config.restart_failed') || '重启失败') + ': ' + e.message);
        }
    }

    async function flushFakeIP() {
        try {
            await api.apiFetch('/cache/fakeip/flush', { method: 'POST' });
            alert(t('config.flush_fakeip_success') || 'FakeIP 缓存已清空');
        } catch (e) { alert((t('common.operation_failed') || '操作失败') + ': ' + e.message); }
    }

    async function flushDNSCache() {
        try {
            await api.apiFetch('/cache/dns/flush', { method: 'POST' });
            alert(t('config.flush_dns_success') || 'DNS 缓存已清空');
        } catch (e) { alert((t('common.operation_failed') || '操作失败') + ': ' + e.message); }
    }

    async function updateGeoDB() {
        try {
            let resp = await api.apiFetch('/providers/geo', { method: 'POST' });
            if (!resp.ok) {
                resp = await api.apiFetch('/configs/geo', { method: 'POST' });
            }
            if (resp.ok) {
                alert(t('config.update_geo_sent') || 'GEO 数据库更新请求已发送');
            } else {
                throw new Error(t('config.update_geo_failed') || '更新失败');
            }
        } catch (e) {
            alert((t('config.update_geo_failed') || '更新失败') + ': ' + e.message);
        }
    }

    async function dnsQuery() {
        const domain = document.getElementById('dns-domain').value.trim();
        const type = document.getElementById('dns-type').value;
        if (!domain) return;
        const resultDiv = document.getElementById('dns-result');
        resultDiv.innerText = t('config.dns_querying') || '查询中...';
        try {
            const resp = await api.apiFetch(`/dns/query?name=${encodeURIComponent(domain)}&type=${type}`);
            const data = await resp.json();
            if (data.Status === 0 && data.Answer) {
                resultDiv.innerText = data.Answer.map(a => a.data).join('\n');
            } else {
                resultDiv.innerText = (t('config.dns_query_failed') || '查询失败') + ': ' + (data.message || t('config.dns_no_record') || '无记录');
            }
        } catch (e) {
            resultDiv.innerText = (t('config.dns_query_failed') || '查询失败') + ': ' + e.message;
        }
    }

    // ---------- 语言和主题 ----------
    function updateLangButton() {
        const btn = document.getElementById('config-lang-toggle');
        if (!btn) return;
        btn.textContent = t('config.lang_toggle');
    }

    function updateThemeButton() {
        const btn = document.getElementById('config-theme-toggle');
        if (!btn) return;
        btn.textContent = t('config.theme_toggle');
    }

    function toggleLanguage() {
        if (!window.i18n) return;
        const newLang = window.i18n.getLanguage() === 'zh' ? 'en' : 'zh';
        window.i18n.setLanguage(newLang);
        window.dispatchEvent(new CustomEvent('languageChanged', { detail: { lang: newLang } }));
        fetchConfig();
        const langToggle = document.getElementById('langToggle');
        if (langToggle) {
            const span = langToggle.querySelector('#currentLang');
            if (span) span.textContent = newLang === 'zh' ? '简' : 'EN';
        }
    }

    function toggleTheme() {
        const current = document.documentElement.getAttribute('data-theme');
        const newTheme = current === 'dark' ? 'light' : 'dark';
        document.documentElement.setAttribute('data-theme', newTheme);
        localStorage.setItem('fluxor-theme', newTheme);
        const moonIcon = document.querySelector('.icon-moon');
        const sunIcon = document.querySelector('.icon-sun');
        if (moonIcon && sunIcon) {
            moonIcon.style.display = newTheme === 'dark' ? 'block' : 'none';
            sunIcon.style.display = newTheme === 'dark' ? 'none' : 'block';
        }
        updateThemeButton();
    }

    // ---------- 渲染 ----------
    function renderForm() {
        if (!container) return;
        const tun = currentConfig?.tun || {};
        container.innerHTML = `
            <div class="card">
                <h3>${t('config.core_config')}</h3>
                <div class="config-row">
                    <label>${t('config.allow_lan')}</label>
                    <label class="toggle-switch">
                        <input type="checkbox" id="cfg-allow-lan">
                        <span class="slider"></span>
                    </label>
                </div>
                <div class="config-row">
                    <label>IPv6</label>
                    <label class="toggle-switch">
                        <input type="checkbox" id="cfg-ipv6">
                        <span class="slider"></span>
                    </label>
                </div>
                <div class="config-row">
                    <label>${t('config.mode')}</label>
                    <select id="cfg-mode">
                        <option value="rule">${t('config.mode_rule')}</option>
                        <option value="global">${t('config.mode_global')}</option>
                        <option value="direct">${t('config.mode_direct')}</option>
                    </select>
                </div>
                <div class="config-row">
                    <label>${t('config.interface_name')}</label>
                    <input type="text" id="cfg-interface-name" placeholder="${t('config.interface_name_placeholder')}">
                </div>
                <div class="config-row" style="margin-top:12px;">
                    <label style="font-weight:600;">${t('config.tun_settings') || 'TUN设置'}</label>
                </div>
                <div class="config-row">
                    <label>${t('config.tun')}</label>
                    <label class="toggle-switch">
                        <input type="checkbox" id="cfg-tun-enable">
                        <span class="slider"></span>
                    </label>
                </div>
                <!-- TUN 高级选项直接显示 -->
                <div class="config-row">
                    <label>${t('config.tun_stack')}</label>
                    <select id="cfg-tun-stack">
                        <option value="System">System</option>
                        <option value="gVisor">gVisor</option>
                        <option value="Mixed">Mixed</option>
                    </select>
                </div>
                <div class="config-row">
                    <label>${t('config.tun_device')}</label>
                    <input type="text" id="cfg-tun-device" placeholder="${t('config.tun_device_auto')}">
                </div>
                <h4>${t('config.port_settings')}</h4>
                <div class="config-row">
                    <label>${t('config.mixed_port')}</label>
                    <input type="number" id="cfg-mixed-port">
                </div>
                <div class="config-row">
                    <label>${t('config.http_port')}</label>
                    <input type="number" id="cfg-http-port">
                </div>
                <div class="config-row">
                    <label>${t('config.socks_port')}</label>
                    <input type="number" id="cfg-socks-port">
                </div>
                <div class="config-row">
                    <label>${t('config.redir_port')}</label>
                    <input type="number" id="cfg-redir-port">
                </div>
                <div class="config-row">
                    <label>${t('config.tproxy_port')}</label>
                    <input type="number" id="cfg-tproxy-port">
                </div>
            </div>

            <div class="card">
                <h3>${t('config.actions')}</h3>
                <div style="display:flex; align-items:center; gap:12px; margin-bottom:12px;">
                    <span id="core-status-indicator" style="display:inline-block; width:12px; height:12px; border-radius:50%; background:var(--text-secondary);"></span>
                    <span id="core-status-text" style="font-size:14px; color:var(--text-secondary);">${t('config.core_checking')}</span>
                </div>
                <div class="button-group">
                    <button id="op-core-toggle" class="btn btn-success">${t('config.start_core')}</button>
                    <button id="op-reload" class="btn btn-secondary">${t('config.reload')}</button>
                    <button id="op-restart" class="btn btn-danger">${t('config.restart')}</button>
                    <button id="op-flush-fakeip" class="btn btn-secondary">${t('config.flush_fakeip')}</button>
                    <button id="op-flush-dns" class="btn btn-secondary">${t('config.flush_dns')}</button>
                    <button id="op-update-geo" class="btn btn-secondary">${t('config.update_geo')}</button>
                </div>
            </div>

            <div class="card">
                <h3>${t('config.interface_settings')}</h3>
                <div style="display:flex; gap:12px; flex-wrap:wrap;">
                    <button id="config-lang-toggle" class="btn" style="background:var(--bg-secondary);color:var(--text-primary);border:1px solid var(--border-color);">${t('config.lang_toggle')}</button>
                    <button id="config-theme-toggle" class="btn" style="background:var(--bg-secondary);color:var(--text-primary);border:1px solid var(--border-color);">${t('config.theme_toggle')}</button>
                </div>
            </div>

            <div class="card">
                <h3>${t('config.dns_query')}</h3>
                <div class="dns-query-box">
                    <input type="text" id="dns-domain" placeholder="${t('config.dns_placeholder')}" class="dns-input">
                    <select id="dns-type">
                        <option value="A">A</option>
                        <option value="AAAA">AAAA</option>
                        <option value="MX">MX</option>
                        <option value="TXT">TXT</option>
                    </select>
                    <button id="dns-query" class="btn">${t('config.dns_query_btn')}</button>
                </div>
                <pre id="dns-result" class="dns-result-pre">${t('config.dns_result_default')}</pre>
            </div>
        `;

        bindEvents();
        populateForm();
        startCoreStatusPolling();
        updateLangButton();
        updateThemeButton();
    }

    function populateForm() {
        if (!currentConfig) return;
        _populating = true;
        try {
            const cfg = currentConfig;
            const tun = cfg.tun || {};
            document.getElementById('cfg-allow-lan').checked = cfg['allow-lan'] || false;
            document.getElementById('cfg-ipv6').checked = cfg['ipv6'] || false;
            document.getElementById('cfg-mode').value = cfg.mode || 'rule';
            document.getElementById('cfg-interface-name').value = cfg['interface-name'] || '';
            document.getElementById('cfg-tun-enable').checked = tun.enable || false;
            // 堆栈值匹配下拉选项（大小写敏感）
            const stackVal = tun.stack || 'System';
            document.getElementById('cfg-tun-stack').value = stackVal;
            document.getElementById('cfg-tun-device').value = tun.device || '';
            document.getElementById('cfg-mixed-port').value = cfg['mixed-port'] || '';
            document.getElementById('cfg-http-port').value = cfg.port || '';
            document.getElementById('cfg-socks-port').value = cfg['socks-port'] || '';
            document.getElementById('cfg-redir-port').value = cfg['redir-port'] || '';
            document.getElementById('cfg-tproxy-port').value = cfg['tproxy-port'] || '';
        } finally {
            _populating = false;
        }
    }

    function bindEvents() {
        ['cfg-allow-lan', 'cfg-ipv6', 'cfg-mode', 'cfg-interface-name',
         'cfg-mixed-port', 'cfg-http-port', 'cfg-socks-port', 'cfg-redir-port', 'cfg-tproxy-port',
         'cfg-tun-enable', 'cfg-tun-stack', 'cfg-tun-device'
        ].forEach(id => {
            const el = document.getElementById(id);
            if (el) {
                el.addEventListener('change', saveCoreDebounced);
                if (el.tagName === 'INPUT' && el.type !== 'checkbox') {
                    el.addEventListener('input', saveCoreDebounced);
                }
            }
        });

        document.getElementById('op-reload').onclick = reloadConfig;
        document.getElementById('op-restart').onclick = restartCore;
        document.getElementById('op-flush-fakeip').onclick = flushFakeIP;
        document.getElementById('op-flush-dns').onclick = flushDNSCache;
        document.getElementById('op-update-geo').onclick = updateGeoDB;
        document.getElementById('dns-query').onclick = dnsQuery;

        document.getElementById('config-lang-toggle').addEventListener('click', toggleLanguage);
        document.getElementById('config-theme-toggle').addEventListener('click', toggleTheme);

        window.addEventListener('languageChanged', updateLangButton);
    }

    async function init() {
        container = document.getElementById('config-content');
        if (!container) return;
        await fetchConfig();
    }

    function destroy() {
        if (saveTimeout) clearTimeout(saveTimeout);
        if (abortController) abortController.abort();
        stopCoreStatusPolling();
        window.removeEventListener('languageChanged', updateLangButton);
    }

    return { init, destroy };
})();