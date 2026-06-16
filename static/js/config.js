// 配置模块：可视化修改核心配置，支持实时保存和操作按钮
window.Config = (function() {
    const BASE = window.BASE_URL || '';

    // 简单的 fetch 封装，统一拼接基础路径
    function apiFetch(path, options) {
        return fetch(BASE + path, options);
    }

    // 端口校验（0 或 1025-65535）
    function validateConfig(payload) {
        const ports = ['mixed-port', 'port', 'socks-port', 'redir-port', 'tproxy-port'];
        for (const key of ports) {
            const val = payload[key];
            if (val !== undefined && val !== 0 && (val < 1025 || val > 65535)) {
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
    let proxySettings = {
        enabled: false,
        url: '',
        token_enabled: false,
        token: '',
        modify_config: false,
        config_backend_url: ''
    };
    let saveTimeout = null;
    let isSaving = false;
    let abortController = null;
    let container = null;

    // ---------- 获取内核配置 ----------
    async function fetchConfig() {
        try {
            const resp = await apiFetch('/configs');
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

    // ---------- 获取代理设置 ----------
    async function fetchProxySettings() {
        try {
            const resp = await apiFetch('/settings');
            if (!resp.ok) return;
            const data = await resp.json();
            proxySettings = data;
            if (document.getElementById('cfg-proxy-enabled')) {
                document.getElementById('cfg-proxy-enabled').checked = proxySettings.enabled;
                document.getElementById('cfg-proxy-url').value = proxySettings.url || '';
                document.getElementById('cfg-token-enabled').checked = proxySettings.token_enabled;
                document.getElementById('cfg-token').value = proxySettings.token || '';
                document.getElementById('cfg-modify-config').checked = proxySettings.modify_config;
                document.getElementById('cfg-config-backend-url').value = proxySettings.config_backend_url || '';
                proxyToggleChanged();
                tokenToggleChanged();
                configToggleChanged();
            }
        } catch (e) {
            console.error('获取代理设置失败:', e);
        }
    }

    // ---------- 收集内核配置表单值 ----------
    function collectCoreFormValues() {
        if (!currentConfig) return null;
        const tun = currentConfig.tun || {};
        return {
            'allow-lan': document.getElementById('cfg-allow-lan').checked,
            mode: document.getElementById('cfg-mode').value,
            'mixed-port': parseInt(document.getElementById('cfg-mixed-port').value) || 7890,
            port: parseInt(document.getElementById('cfg-http-port').value) || 0,
            'socks-port': parseInt(document.getElementById('cfg-socks-port').value) || 0,
            'redir-port': parseInt(document.getElementById('cfg-redir-port').value) || 0,
            'tproxy-port': parseInt(document.getElementById('cfg-tproxy-port').value) || 0,
            'interface-name': document.getElementById('cfg-interface-name').value || null,
            tun: {
                enable: document.getElementById('cfg-tun-enable').checked,
                stack: document.getElementById('cfg-tun-stack').value,
                device: document.getElementById('cfg-tun-device').value || null,
                'auto-route': document.getElementById('cfg-tun-auto-route').checked,
                'dns-hijack': document.getElementById('cfg-tun-dns-hijack').value ?
                    document.getElementById('cfg-tun-dns-hijack').value.split(',').map(s => s.trim()) : null,
                mtu: (() => {
                    const mtuVal = parseInt(document.getElementById('cfg-tun-mtu').value);
                    return isNaN(mtuVal) || mtuVal <= 0 ? undefined : mtuVal;
                })()
            }
        };
    }

    // ---------- 保存内核配置（防抖） ----------
    function saveCoreDebounced() {
        if (saveTimeout) clearTimeout(saveTimeout);
        saveTimeout = setTimeout(saveCore, 500);
    }

    async function saveCore() {
        if (!currentConfig || isSaving) return;
        const payload = collectCoreFormValues();
        if (!payload || !validateConfig(payload)) return;

        if (abortController) abortController.abort();
        abortController = new AbortController();
        isSaving = true;

        try {
            const resp = await apiFetch('/configs', {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
                signal: abortController.signal
            });
            if (resp.ok) {
                await fetchConfig();
            } else {
                const errText = await resp.text();
                console.error('保存失败响应:', errText);
            }
        } catch (err) {
            if (err.name !== 'AbortError') console.error('保存失败:', err);
        } finally {
            isSaving = false;
            abortController = null;
        }
    }

    // ---------- 保存代理设置 ----------
    async function saveProxySettings() {
        const enabled = document.getElementById('cfg-proxy-enabled').checked;
        const url = document.getElementById('cfg-proxy-url').value.trim();
        const tokenEnabled = document.getElementById('cfg-token-enabled').checked;
        const token = document.getElementById('cfg-token').value.trim();
        const modifyConfig = document.getElementById('cfg-modify-config').checked;
        const backendUrl = document.getElementById('cfg-config-backend-url').value.trim();

        if (enabled && !url) { alert('请输入代理地址'); return; }
        if (enabled && url && !/^https?:\/\//.test(url)) { alert('代理地址格式不正确'); return; }
        if (modifyConfig && backendUrl && !/^https?:\/\//.test(backendUrl)) { alert('后端地址格式不正确'); return; }

        try {
            const resp = await apiFetch('/settings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    enabled,
                    url,
                    token_enabled: tokenEnabled,
                    token,
                    modify_config: modifyConfig,
                    config_backend_url: backendUrl
                })
            });
            const result = await resp.json();
            if (resp.ok) {
                alert(result.message || '代理设置已保存');
                await fetchProxySettings();
            } else {
                alert('保存失败: ' + (result.message || result.error || ''));
            }
        } catch (e) {
            alert('网络错误: ' + e.message);
        }
    }

    // ---------- 操作按钮 ----------
    // 热重载配置（不中断连接）
    async function reloadConfig() {
        try {
            const resp = await apiFetch('/configs', { method: 'PUT' });
            if (resp.ok) {
                await fetchConfig();
                alert('配置已热重载');
            } else {
                throw new Error('重载失败');
            }
        } catch (e) {
            alert('重载失败: ' + e.message);
        }
    }

    // 真正重启内核（会断开所有连接）
    async function restartCore() {
        if (!confirm(window.i18n?.t('config.confirm_restart') || '确定要重启内核吗？所有连接将断开。')) return;
        try {
            const resp = await apiFetch('/restart', { method: 'POST' });
            if (!resp.ok) throw new Error('重启失败');
            alert('重启指令已发送');
        } catch (e) {
            alert('重启失败: ' + e.message);
        }
    }

    async function flushFakeIP() {
        try {
            await apiFetch('/cache/fakeip/flush', { method: 'POST' });
            alert('FakeIP 缓存已清空');
        } catch (e) { alert('操作失败: ' + e.message); }
    }

    async function flushDNSCache() {
        try {
            await apiFetch('/cache/dns/flush', { method: 'POST' });
            alert('DNS 缓存已清空');
        } catch (e) { alert('操作失败: ' + e.message); }
    }

    async function updateGeoDB() {
        try {
            await apiFetch('/providers/geo', { method: 'POST' });
            alert('GEO 数据库更新请求已发送');
        } catch (e) {
            await apiFetch('/configs/geo', { method: 'POST' }).catch(() => {});
            alert('GEO 更新请求已发送');
        }
    }

    async function dnsQuery() {
        const domain = document.getElementById('dns-domain').value.trim();
        const type = document.getElementById('dns-type').value;
        if (!domain) return;
        const resultDiv = document.getElementById('dns-result');
        resultDiv.innerText = '查询中...';
        try {
            const resp = await apiFetch(`/dns/query?name=${encodeURIComponent(domain)}&type=${type}`);
            const data = await resp.json();
            if (data.Status === 0 && data.Answer) {
                resultDiv.innerText = data.Answer.map(a => a.data).join('\n');
            } else {
                resultDiv.innerText = '查询失败: ' + (data.message || '无记录');
            }
        } catch (e) {
            resultDiv.innerText = '查询失败: ' + e.message;
        }
    }

    // ---------- 代理开关联动 ----------
    function proxyToggleChanged() {
        const enabled = document.getElementById('cfg-proxy-enabled').checked;
        document.getElementById('cfg-proxy-url').disabled = !enabled;
    }
    function tokenToggleChanged() {
        const enabled = document.getElementById('cfg-token-enabled').checked;
        document.getElementById('cfg-token').disabled = !enabled;
    }
    function configToggleChanged() {
        const enabled = document.getElementById('cfg-modify-config').checked;
        document.getElementById('cfg-config-backend-url').disabled = !enabled;
    }

    // ---------- 渲染界面 ----------
    function renderForm() {
        if (!container) return;
        const t = (window.i18n && window.i18n.t) || (key => key);
        const tun = currentConfig?.tun || {};
        container.innerHTML = `
            <div class="card">
                <h3>${t('config.proxy_settings')}</h3>
                <div class="config-row">
                    <label>${t('config.enable_proxy')}</label>
                    <label class="toggle-switch">
                        <input type="checkbox" id="cfg-proxy-enabled">
                        <span class="slider"></span>
                    </label>
                </div>
                <div class="config-row">
                    <label for="cfg-proxy-url">${t('config.proxy_url')}</label>
                    <input type="text" id="cfg-proxy-url" placeholder="${t('config.proxy_url_placeholder')}" disabled>
                </div>
                <div class="config-row">
                    <label>${t('config.enable_token')}</label>
                    <label class="toggle-switch">
                        <input type="checkbox" id="cfg-token-enabled">
                        <span class="slider"></span>
                    </label>
                </div>
                <div class="config-row">
                    <label for="cfg-token">${t('config.github_token')}</label>
                    <input type="password" id="cfg-token" placeholder="${t('config.token_placeholder')}" disabled>
                    <a href="https://github.com/settings/personal-access-tokens" target="_blank" class="token-link">${t('config.token_link')}</a>
                </div>
                <div class="config-row">
                    <label>${t('config.modify_metacubexd')}</label>
                    <label class="toggle-switch">
                        <input type="checkbox" id="cfg-modify-config">
                        <span class="slider"></span>
                    </label>
                </div>
                <div class="config-row">
                    <label for="cfg-config-backend-url">${t('config.metacubexd_backend_url')}</label>
                    <input type="text" id="cfg-config-backend-url" placeholder="${t('config.metacubexd_url_placeholder')}" disabled>
                </div>
                <button id="save-proxy" class="btn">${t('config.save_proxy')}</button>
            </div>

            <div class="card">
                <h3>${t('config.core_config')}</h3>
                <div class="config-row"><label>${t('config.allow_lan')}</label><input type="checkbox" id="cfg-allow-lan"></div>
                <div class="config-row"><label>${t('config.mode')}</label><select id="cfg-mode">
                    <option value="rule">${t('config.mode_rule')}</option>
                    <option value="global">${t('config.mode_global')}</option>
                    <option value="direct">${t('config.mode_direct')}</option>
                </select></div>
                <div class="config-row"><label>${t('config.interface_name')}</label><input type="text" id="cfg-interface-name" placeholder="${t('config.interface_name_placeholder')}"></div>
                <h4>${t('config.tun')}</h4>
                <div class="config-row"><label>${t('config.tun_enable')}</label><input type="checkbox" id="cfg-tun-enable"></div>
                <div class="config-row"><label>${t('config.tun_stack')}</label><select id="cfg-tun-stack">
                    <option value="system">system</option>
                    <option value="gvisor">gvisor</option>
                    <option value="mixed">mixed</option>
                </select></div>
                <div class="config-row"><label>${t('config.tun_device')}</label><input type="text" id="cfg-tun-device" placeholder="${t('config.tun_device_auto')}"></div>
                <div class="config-row"><label>${t('config.auto_route')}</label><input type="checkbox" id="cfg-tun-auto-route"></div>
                <div class="config-row"><label>${t('config.dns_hijack')}</label><input type="text" id="cfg-tun-dns-hijack" placeholder="${t('config.dns_hijack_placeholder')}"></div>
                <div class="config-row"><label>${t('config.mtu')}</label><input type="number" id="cfg-tun-mtu" placeholder="${t('config.mtu_default')}"></div>
                <h4>${t('config.port_settings')}</h4>
                <div class="config-row"><label>${t('config.mixed_port')}</label><input type="number" id="cfg-mixed-port"></div>
                <div class="config-row"><label>${t('config.http_port')}</label><input type="number" id="cfg-http-port"></div>
                <div class="config-row"><label>${t('config.socks_port')}</label><input type="number" id="cfg-socks-port"></div>
                <div class="config-row"><label>${t('config.redir_port')}</label><input type="number" id="cfg-redir-port"></div>
                <div class="config-row"><label>${t('config.tproxy_port')}</label><input type="number" id="cfg-tproxy-port"></div>
            </div>

            <div class="card">
                <h3>${t('config.actions')}</h3>
                <div class="button-group">
                    <button id="op-reload" class="btn">${t('config.reload')}</button>
                    <button id="op-restart" class="btn btn-danger">${t('config.restart')}</button>
                    <button id="op-flush-fakeip" class="btn">${t('config.flush_fakeip')}</button>
                    <button id="op-flush-dns" class="btn">${t('config.flush_dns')}</button>
                    <button id="op-update-geo" class="btn">${t('config.update_geo')}</button>
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
        fetchProxySettings();
    }

    function bindEvents() {
        // 自动保存
        ['cfg-allow-lan', 'cfg-mode', 'cfg-interface-name', 'cfg-tun-enable', 'cfg-tun-stack', 'cfg-tun-device',
         'cfg-tun-auto-route', 'cfg-tun-dns-hijack', 'cfg-tun-mtu', 'cfg-mixed-port', 'cfg-http-port',
         'cfg-socks-port', 'cfg-redir-port', 'cfg-tproxy-port'].forEach(id => {
            const el = document.getElementById(id);
            if (el) {
                el.addEventListener('change', saveCoreDebounced);
                if (el.tagName === 'INPUT' && el.type !== 'checkbox') el.addEventListener('input', saveCoreDebounced);
            }
        });

        // 操作按钮
        document.getElementById('op-reload').onclick = reloadConfig;
        document.getElementById('op-restart').onclick = restartCore;
        document.getElementById('op-flush-fakeip').onclick = flushFakeIP;
        document.getElementById('op-flush-dns').onclick = flushDNSCache;
        document.getElementById('op-update-geo').onclick = updateGeoDB;
        document.getElementById('dns-query').onclick = dnsQuery;

        // 代理保存
        document.getElementById('save-proxy').onclick = saveProxySettings;

        // 代理开关
        document.getElementById('cfg-proxy-enabled').addEventListener('change', proxyToggleChanged);
        document.getElementById('cfg-token-enabled').addEventListener('change', tokenToggleChanged);
        document.getElementById('cfg-modify-config').addEventListener('change', configToggleChanged);
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/[&<>]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[m]));
    }

    async function init() {
        container = document.getElementById('config-content');
        if (!container) return;
        await fetchConfig();
    }

    function destroy() {
        if (saveTimeout) clearTimeout(saveTimeout);
        if (abortController) abortController.abort();
    }

    return { init, destroy };
})();