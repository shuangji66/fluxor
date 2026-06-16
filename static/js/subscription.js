// 订阅中心模块
window.Subscription = (function() {
    // ---------- 工具函数 ----------
    function escapeHtml(text) {
        if (!text) return '';
        const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
        return String(text).replace(/[&<>"']/g, m => map[m]);
    }

    // ---------- 状态变量 ----------
    let currentConfig = { subscriptions: [] };
    let editingIndex = -1;
    let container = null;

    // ---------- API 封装 ----------
    const BASE = window.FLUXOR_BASE || '';
    function apiFetch(path, options) {
        return fetch(BASE + path, options);
    }

    // ---------- 初始化 ----------
    function init() {
        container = document.getElementById('subscription-content');
        if (!container) return;
        render();
        loadConfig();
    }

    // ---------- 加载配置 ----------
    async function loadConfig() {
        try {
            const resp = await apiFetch('/subscribe/config');
            if (!resp.ok) throw new Error('HTTP ' + resp.status);
            const cfg = await resp.json();
            currentConfig = cfg;
            // 回填表单
            document.getElementById('subProxyPort').value = cfg.proxy_port || 7890;
            document.getElementById('subPanelPort').value = cfg.panel_port || 9090;
            document.getElementById('subPanelSecret').value = cfg.panel_secret || '';
            document.getElementById('subRuleGroup').value = cfg.rule_group || 'none';
            document.getElementById('subPrefixSwitch').checked = cfg.prefix_switch || false;
            togglePrefixMode();
            renderSubList(currentConfig.subscriptions || []);
        } catch (e) {
            console.error('[Subscription] 加载配置失败:', e);
            currentConfig = { subscriptions: [] };
            renderSubList([]);
        }
    }

    // ---------- 渲染主界面 ----------
    function render() {
        container.innerHTML = `
            <div class="card">
                <h3>订阅中心</h3>
                <div class="config-section">
                    <label>代理端口</label>
                    <input type="number" id="subProxyPort" placeholder="例如 7890">
                </div>
                <div class="config-section">
                    <label>面板端口</label>
                    <input type="number" id="subPanelPort" placeholder="例如 9090">
                </div>
                <div class="config-section">
                    <label>面板密钥</label>
                    <input type="text" id="subPanelSecret" placeholder="面板密钥">
                </div>
                <div class="config-section">
                    <label>规则集</label>
                    <select id="subRuleGroup">
                        <option value="none" disabled>请选择规则集</option>
                        <option value="lite">基础 (Lite)</option>
                        <option value="base">标准 (Base)</option>
                        <option value="full">完整 (Full)</option>
                    </select>
                </div>
                <div class="config-section setting-row">
                    <label for="subPrefixSwitch">启用节点前缀</label>
                    <label class="toggle-switch">
                        <input type="checkbox" id="subPrefixSwitch">
                        <span class="slider"></span>
                    </label>
                </div>

                <div style="display:flex; justify-content:space-between; align-items:center; margin-top: 20px;">
                    <h4>订阅列表</h4>
                    <button class="btn" id="addSubBtn">+ 添加订阅</button>
                </div>
                <div id="subList"></div>

                <div style="margin-top: 20px; display: flex; gap: 10px;">
                    <button class="btn" id="saveSubConfigBtn">💾 保存设置</button>
                    <button class="btn btn-primary" id="saveApplyBtn">🚀 保存并应用</button>
                </div>
            </div>

            <!-- 订阅编辑弹窗 -->
            <div class="modal-overlay" id="subModal">
                <div class="modal">
                    <button type="button" class="modal-close">&times;</button>
                    <h2 id="subModalTitle">添加订阅</h2>
                    <div class="modal-section">
                        <label>订阅名称</label>
                        <input type="text" id="subName" placeholder="例如：我的节点">
                    </div>
                    <div class="modal-section">
                        <label>订阅链接</label>
                        <input type="text" id="subUrl" placeholder="https://example.com/sub">
                    </div>
                    <div class="modal-section">
                        <label>更新间隔（秒）</label>
                        <input type="number" id="subUpdateInterval" placeholder="3600">
                    </div>
                    <div class="modal-section">
                        <label>健康检查间隔（秒）</label>
                        <input type="number" id="subHealthInterval" placeholder="300">
                    </div>
                    <div class="modal-section" id="prefixSection" style="display:none;">
                        <label>节点前缀</label>
                        <input type="text" id="subPrefix" placeholder="[Proxy]">
                    </div>
                    <p class="modal-hint">添加后将保存在本页列表，需点击主界面按钮持久化。</p>
                    <div class="modal-actions">
                        <button type="button" class="btn btn-cancel">取消</button>
                        <button type="button" class="btn btn-primary" id="saveSubBtn">保存到列表</button>
                    </div>
                </div>
            </div>
        `;

        // 绑定事件（重要：在 render 后绑定，否则 DOM 不存在）
        bindEvents();
    }

    // ---------- 事件绑定 ----------
    function bindEvents() {
        // 添加订阅按钮
        document.getElementById('addSubBtn').onclick = () => openSubModal(-1);
        // 保存设置
        document.getElementById('saveSubConfigBtn').onclick = saveConfig;
        // 保存并应用
        document.getElementById('saveApplyBtn').onclick = saveAndApply;

        // 弹窗内按钮
        const modal = document.getElementById('subModal');
        modal.querySelector('.modal-close').onclick = closeSubModal;
        modal.querySelector('.btn-cancel').onclick = closeSubModal;
        modal.querySelector('#saveSubBtn').onclick = saveSub;

        // 点击遮罩关闭
        modal.addEventListener('click', function(e) {
            if (e.target === this) closeSubModal();
        });

        // 前缀开关变化
        document.getElementById('subPrefixSwitch').addEventListener('change', togglePrefixMode);
    }

    // ---------- UI 逻辑 ----------
    function togglePrefixMode() {
        const enabled = document.getElementById('subPrefixSwitch').checked;
        const prefixSection = document.getElementById('prefixSection');
        if (prefixSection) prefixSection.style.display = enabled ? 'block' : 'none';
    }

    function renderSubList(subs) {
        const list = document.getElementById('subList');
        if (!list) return;
        if (!subs || subs.length === 0) {
            list.innerHTML = '<p>暂无订阅</p>';
            return;
        }
        const prefixEnabled = document.getElementById('subPrefixSwitch').checked;
        list.innerHTML = subs.map((sub, idx) => `
            <div class="sub-item">
                <div class="info">
                    <strong>${escapeHtml(sub.name)}</strong><br>
                    <small>${escapeHtml(sub.url)}</small><br>
                    <small>更新间隔: ${sub.update_interval}s | 健康检查: ${sub.health_interval}s</small>
                    ${prefixEnabled ? `<br><small>前缀: ${escapeHtml(sub.prefix || '')}</small>` : ''}
                </div>
                <div class="actions">
                    <button type="button" class="btn-edit" data-index="${idx}">✏️</button>
                    <button type="button" class="btn-delete" data-index="${idx}">🗑️</button>
                </div>
            </div>
        `).join('');

        // 绑定编辑/删除事件
        document.querySelectorAll('.btn-edit').forEach(btn => {
            btn.onclick = (e) => editSub(parseInt(e.currentTarget.dataset.index));
        });
        document.querySelectorAll('.btn-delete').forEach(btn => {
            btn.onclick = (e) => deleteSub(parseInt(e.currentTarget.dataset.index));
        });
    }

    function openSubModal(index = -1) {
        editingIndex = index;
        const modal = document.getElementById('subModal');
        if (!modal) return;

        const prefixEnabled = document.getElementById('subPrefixSwitch').checked;
        document.getElementById('prefixSection').style.display = prefixEnabled ? 'block' : 'none';

        if (index >= 0) {
            document.getElementById('subModalTitle').textContent = '编辑订阅';
            const sub = currentConfig.subscriptions[index];
            document.getElementById('subName').value = sub.name || '';
            document.getElementById('subUrl').value = sub.url || '';
            document.getElementById('subUpdateInterval').value = sub.update_interval || 3600;
            document.getElementById('subHealthInterval').value = sub.health_interval || 300;
            document.getElementById('subPrefix').value = sub.prefix || '';
        } else {
            document.getElementById('subModalTitle').textContent = '添加订阅';
            document.getElementById('subName').value = '';
            document.getElementById('subUrl').value = '';
            document.getElementById('subUpdateInterval').value = '';
            document.getElementById('subHealthInterval').value = '';
            document.getElementById('subPrefix').value = '';
        }
        modal.classList.add('active');
    }

    function closeSubModal() {
        document.getElementById('subModal').classList.remove('active');
    }

    function saveSub() {
        const name = document.getElementById('subName').value.trim();
        const url = document.getElementById('subUrl').value.trim();
        if (!name || !url) { alert('名称和链接不能为空'); return; }

        if (!currentConfig.subscriptions) currentConfig.subscriptions = [];
        const updateInterval = parseInt(document.getElementById('subUpdateInterval').value) || 3600;
        const healthInterval = parseInt(document.getElementById('subHealthInterval').value) || 300;
        const prefix = document.getElementById('subPrefix').value.trim();

        const sub = { name, url, update_interval: updateInterval, health_interval: healthInterval, prefix };
        if (editingIndex >= 0 && editingIndex < currentConfig.subscriptions.length) {
            currentConfig.subscriptions[editingIndex] = sub;
        } else {
            currentConfig.subscriptions.push(sub);
        }
        renderSubList(currentConfig.subscriptions);
        closeSubModal();
    }

    function editSub(index) { openSubModal(index); }
    function deleteSub(index) {
        if (!confirm('确定删除该订阅？')) return;
        currentConfig.subscriptions.splice(index, 1);
        renderSubList(currentConfig.subscriptions);
    }

    // ---------- 持久化 ----------
    async function saveConfig() {
        if (!validate()) return;
        await postConfig('/subscribe/config', '配置已保存');
    }

    async function saveAndApply() {
        if (!validate()) return;
        await postConfig('/subscribe/generate', '配置已保存并应用');
    }

    function validate() {
        const proxyPort = document.getElementById('subProxyPort').value.trim();
        const panelPort = document.getElementById('subPanelPort').value.trim();
        if (!proxyPort || !panelPort) { alert('请填写代理端口和面板端口'); return false; }
        if (document.getElementById('subRuleGroup').value === 'none') { alert('请选择规则集'); return false; }
        return true;
    }

    async function postConfig(url, successMsg) {
        const payload = {
            proxy_port: parseInt(document.getElementById('subProxyPort').value),
            panel_port: parseInt(document.getElementById('subPanelPort').value),
            panel_secret: document.getElementById('subPanelSecret').value.trim(),
            rule_group: document.getElementById('subRuleGroup').value,
            prefix_switch: document.getElementById('subPrefixSwitch').checked,
            subscriptions: currentConfig.subscriptions
        };
        try {
            const resp = await apiFetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
            const result = await resp.json();
            if (resp.ok && result.status === 'ok') {
                alert(result.message || successMsg);
            } else {
                alert('操作失败: ' + (result.message || result.error || ''));
            }
        } catch (e) {
            alert('网络错误: ' + e.message);
        }
    }

    return { init };
})();