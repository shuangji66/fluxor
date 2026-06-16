const urlPattern = /^https?:\/\/((\d{1,3}\.){3}\d{1,3}|([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}):([1-9]\d{0,3}|[1-5]\d{4}|6[0-4]\d{3}|65[0-4]\d{2}|655[0-2]\d|6553[0-5])$/;

function showMessage(text, isSuccess) {
    const msg = document.getElementById('msg');
    if (!msg) return;
    msg.style.display = 'block';
    msg.textContent = text;
    msg.className = 'message ' + (isSuccess ? 'success' : 'error');
}
function hideMessage() {
    const msg = document.getElementById('msg');
    if (msg) msg.style.display = 'none';
}

async function updateResource(btnId, url) {
    const btn = document.getElementById(btnId);
    if (!btn) return;
    btn.disabled = true;
    btn.textContent = '⏳';
    hideMessage();
    try {
        const resp = await fetch(url, { method: 'POST' });
        const data = await resp.json();
        showMessage(data.success ? '✅ ' + data.message : '❌ ' + data.message, data.success);
    } catch (err) {
        showMessage('❌ 请求失败: ' + err.message, false);
    } finally {
        btn.disabled = false;
        btn.textContent = '🔄';
    }
}

let currentProxyEnabled = false, currentProxyURL = '', currentTokenEnabled = false, currentToken = '', currentConfigEnabled = false, currentBackendURL = '';

async function loadSettings() {
    try {
        const resp = await fetch(baseURL + '/settings');
        const data = await resp.json();
        currentProxyEnabled = data.enabled || false;
        currentProxyURL = data.url || '';
        currentTokenEnabled = data.token_enabled || false;
        currentToken = data.token || '';
        currentConfigEnabled = data.modify_config || false;
        currentBackendURL = data.config_backend_url || '';
        document.getElementById('proxyToggle').checked = currentProxyEnabled;
        document.getElementById('proxyUrl').value = currentProxyURL;
        document.getElementById('tokenToggle').checked = currentTokenEnabled;
        document.getElementById('githubToken').value = currentToken;
        document.getElementById('configToggle').checked = currentConfigEnabled;
        document.getElementById('backendUrl').value = currentBackendURL;
        proxyToggleChanged(); tokenToggleChanged(); configToggleChanged();
    } catch(e) { console.error('加载设置失败', e); }
}

function proxyToggleChanged() {
    const input = document.getElementById('proxyUrl');
    if (input) {
        input.disabled = !document.getElementById('proxyToggle')?.checked;
        if (!input.disabled && !input.value) input.value = currentProxyURL;
        else if (input.disabled) input.value = '';
    }
}
function tokenToggleChanged() {
    const input = document.getElementById('githubToken');
    if (input) {
        input.disabled = !document.getElementById('tokenToggle')?.checked;
        if (!input.disabled && !input.value) input.value = currentToken;
        else if (input.disabled) input.value = '';
    }
}
function configToggleChanged() {
    const input = document.getElementById('backendUrl');
    if (input) {
        input.disabled = !document.getElementById('configToggle')?.checked;
        if (!input.disabled && !input.value) input.value = currentBackendURL;
        else if (input.disabled) input.value = '';
    }
}
function openSettings() {
    loadSettings();
    document.getElementById('settingsModal')?.classList.add('active');
}
function closeSettings() {
    document.getElementById('settingsModal')?.classList.remove('active');
}
async function saveSettings() {
    const enabled = document.getElementById('proxyToggle')?.checked;
    const url = document.getElementById('proxyUrl')?.value.trim();
    const tokenEnabled = document.getElementById('tokenToggle')?.checked;
    const token = document.getElementById('githubToken')?.value.trim();
    const configEnabled = document.getElementById('configToggle')?.checked;
    const backendUrl = document.getElementById('backendUrl')?.value.trim();
    if (enabled && !url) return alert('请输入代理地址');
    if (enabled && url && !urlPattern.test(url)) return alert('代理地址格式不正确');
    if (configEnabled && !backendUrl) return alert('请输入后端地址');
    if (configEnabled && backendUrl && !urlPattern.test(backendUrl)) return alert('后端地址格式不正确');
    try {
        const resp = await fetch(baseURL + '/settings', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({enabled, url, token_enabled: tokenEnabled, token, modify_config: configEnabled, config_backend_url: backendUrl})
        });
        const result = await resp.json();
        if (resp.ok) {
            currentProxyEnabled = enabled; currentProxyURL = url; currentTokenEnabled = tokenEnabled; currentToken = token; currentConfigEnabled = configEnabled; currentBackendURL = backendUrl;
            closeSettings();
            showMessage(result.message || '✅ 设置已保存', true);
        } else {
            showMessage('❌ 保存失败: ' + (result.message || result.error || ''), false);
        }
    } catch(e) { showMessage('❌ 保存失败: ' + e.message, false); }
}

document.addEventListener('click', function(e) {
    if (e.target.classList.contains('modal-overlay') && e.target.classList.contains('active')) {
        e.target.classList.remove('active');
    }
});

let coreRunning = false;
async function updateCoreStatus() {
    try {
        const resp = await fetch(baseURL + '/core/status');
        const data = await resp.json();
        coreRunning = data.running;
        const btn = document.getElementById('coreStartStopBtn');
        if (btn) {
            if (coreRunning) {
                btn.textContent = '⏹️ 停止内核';
                btn.className = 'action-btn core-btn stop';
            } else {
                btn.textContent = '▶️ 启动内核';
                btn.className = 'action-btn core-btn start';
            }
        }
    } catch(e) { console.error('获取内核状态失败', e); }
}
async function toggleCore() {
    const btn = document.getElementById('coreStartStopBtn');
    if (!btn) return;
    btn.disabled = true;
    const url = coreRunning ? baseURL + '/core/stop' : baseURL + '/core/start';
    try {
        const resp = await fetch(url, { method: 'POST' });
        const result = await resp.json();
        if (resp.ok && result.status === 'ok') {
            showMessage(result.message, true);
            setTimeout(updateCoreStatus, 1500);
        } else {
            showMessage('操作失败: ' + (result.message || ''), false);
        }
    } catch(e) { showMessage('请求失败: ' + e.message, false); }
    finally { btn.disabled = false; }
}
async function restartCore() {
    const btn = document.querySelector('.restart-btn');
    if (btn) btn.disabled = true;
    try {
        const resp = await fetch(baseURL + '/core/restart', { method: 'POST' });
        if (resp.ok) {
            showMessage('重启指令已发送', true);
            setTimeout(updateCoreStatus, 2000);
        } else {
            const msg = await resp.text();
            showMessage('重启失败: ' + msg, false);
        }
    } catch (err) {
        showMessage('请求失败: ' + err.message, false);
    } finally {
        if (btn) btn.disabled = false;
    }
}