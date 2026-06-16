window.i18n = (function() {
    let currentLang = localStorage.getItem('lang') || 'zh';
    
    const translations = {
        zh: {
            // 通用
            'common.loading': '加载中...',
            'common.error': '错误',
            'common.success': '成功',
            'common.confirm': '确认',
            'common.cancel': '取消',
            'common.delete': '删除',
            'common.edit': '编辑',
            'common.save': '保存',
            'common.refresh': '刷新',
            'common.close': '关闭',
            
            // 侧边栏
            'nav.overview': '概览',
            'nav.proxies': '代理',
            'nav.proxies_desc': '代理选择',
            'nav.rules': '规则',
            'nav.rules_desc': '规则管理',
            'nav.subscriptions': '订阅',
            'nav.subscriptions_desc': '订阅管理',
            'nav.connections': '连接',
            'nav.connections_desc': '活跃连接',
            'nav.logs': '日志',
            'nav.logs_desc': '系统日志',
            'nav.config': '配置',
            'nav.config_desc': '配置管理',
            
            // 概览模块
            'overview.title': '仪表盘',
            'overview.core_version': '内核版本',
            'overview.upload_speed': '上传速度',
            'overview.download_speed': '下载速度',
            'overview.upload_total': '总上传量',
            'overview.download_total': '总下载量',
            'overview.memory_usage': '内存占用',
            'overview.active_connections': '活跃连接',
            'overview.traffic_trend': '流量趋势',
            'overview.upload': '上传',
            'overview.download': '下载',
            
            // 代理模块
            'proxies.title': '代理组',
            'proxies.current': '当前选择',
            'proxies.test': '测速',
            'proxies.testing': '测速中',
            'proxies.timeout': '超时',
            'proxies.error': '错误',
            'proxies.test_all': '全部测速',
            'proxies.test_complete': '测速完成',
            'proxies.testing_all': '正在测速所有节点...',
            'proxies.no_groups': '没有可用的代理组',
            'proxies.switched': '已切换',
            'proxies.switch_failed': '切换失败',
            'proxies.empty': '暂无代理组',
            'proxies.load_failed': '加载代理失败',
            
            // 规则模块
            'rules.title': '规则',
            'rules.type': '类型',
            'rules.payload': '内容',
            'rules.proxy': '代理',
            'rules.total': '总计',
            'rules.rules_count': '���规则',
            'rules.search_placeholder': '搜索规则...',
            'rules.providers_title': '规则提供商',
            'rules.update_provider': '更新此提供商',
            'rules.update_all_btn': '全部更新',
            'rules.provider_update_success': '{name} 更新成功',
            'rules.provider_update_failed': '{name} 更新失败',
            'rules.no_providers': '暂无规则提供商',
            'rules.updating_providers': '正在并发更新 {count} 个提供商...',
            'rules.batch_update_complete': '批量更新完成',
            'rules.unknown_time': '未知',
            'rules.load_failed': '加载规则失败',
            
            // 连接模块
            'connections.title': '连接管理',
            'connections.host': '主机',
            'connections.port': '端口',
            'connections.rule': '规则',
            'connections.chain': '链路',
            'connections.upload_speed': '上传',
            'connections.download_speed': '下载',
            'connections.action': '操作',
            'connections.close': '断开',
            'connections.close_all': '全部断开',
            'connections.pause': '暂停',
            'connections.resume': '继续',
            'connections.confirm_close_all': '确定要断开所有连接吗？',
            'connections.close_failed': '断开失败',
            'connections.close_all_success': '已断开所有连接',
            'connections.close_all_failed': '批量断开失败',
            'connections.empty': '暂无活跃连接',
            'connections.loading': '加载中...',
            'connections.search_placeholder': '搜索连接...',
            
            // 日志模块
            'logs.title': '日志查看',
            'logs.pause': '暂停',
            'logs.resume': '继续',
            'logs.clear': '清空',
            'logs.ws_connected': 'WebSocket 已连接',
        },
        en: {
            // Common
            'common.loading': 'Loading...',
            'common.error': 'Error',
            'common.success': 'Success',
            'common.confirm': 'Confirm',
            'common.cancel': 'Cancel',
            'common.delete': 'Delete',
            'common.edit': 'Edit',
            'common.save': 'Save',
            'common.refresh': 'Refresh',
            'common.close': 'Close',
            
            // Navigation
            'nav.overview': 'Overview',
            'nav.proxies': 'Proxies',
            'nav.proxies_desc': 'Proxy Selection',
            'nav.rules': 'Rules',
            'nav.rules_desc': 'Rules Management',
            'nav.subscriptions': 'Subscriptions',
            'nav.subscriptions_desc': 'Subscription Management',
            'nav.connections': 'Connections',
            'nav.connections_desc': 'Active Connections',
            'nav.logs': 'Logs',
            'nav.logs_desc': 'System Logs',
            'nav.config': 'Config',
            'nav.config_desc': 'Configuration',
            
            // Overview
            'overview.title': 'Dashboard',
            'overview.core_version': 'Core Version',
            'overview.upload_speed': 'Upload Speed',
            'overview.download_speed': 'Download Speed',
            'overview.upload_total': 'Total Upload',
            'overview.download_total': 'Total Download',
            'overview.memory_usage': 'Memory Usage',
            'overview.active_connections': 'Active Connections',
            'overview.traffic_trend': 'Traffic Trend',
            'overview.upload': 'Upload',
            'overview.download': 'Download',
            
            // Proxies
            'proxies.title': 'Proxy Groups',
            'proxies.current': 'Current',
            'proxies.test': 'Test',
            'proxies.testing': 'Testing',
            'proxies.timeout': 'Timeout',
            'proxies.error': 'Error',
            'proxies.test_all': 'Test All',
            'proxies.test_complete': 'Speed test completed',
            'proxies.testing_all': 'Testing all proxies...',
            'proxies.no_groups': 'No proxy groups available',
            'proxies.switched': 'Switched to',
            'proxies.switch_failed': 'Switch failed',
            'proxies.empty': 'No proxy groups',
            'proxies.load_failed': 'Failed to load proxies',
            
            // Rules
            'rules.title': 'Rules',
            'rules.type': 'Type',
            'rules.payload': 'Payload',
            'rules.proxy': 'Proxy',
            'rules.total': 'Total',
            'rules.rules_count': 'rules',
            'rules.search_placeholder': 'Search rules...',
            'rules.providers_title': 'Rule Providers',
            'rules.update_provider': 'Update this provider',
            'rules.update_all_btn': 'Update All',
            'rules.provider_update_success': '{name} updated',
            'rules.provider_update_failed': '{name} update failed',
            'rules.no_providers': 'No rule providers',
            'rules.updating_providers': 'Updating {count} providers concurrently...',
            'rules.batch_update_complete': 'Batch update completed',
            'rules.unknown_time': 'Unknown',
            'rules.load_failed': 'Failed to load rules',
            
            // Connections
            'connections.title': 'Connections',
            'connections.host': 'Host',
            'connections.port': 'Port',
            'connections.rule': 'Rule',
            'connections.chain': 'Chain',
            'connections.upload_speed': 'Upload',
            'connections.download_speed': 'Download',
            'connections.action': 'Action',
            'connections.close': 'Close',
            'connections.close_all': 'Close All',
            'connections.pause': 'Pause',
            'connections.resume': 'Resume',
            'connections.confirm_close_all': 'Are you sure to close all connections?',
            'connections.close_failed': 'Close failed',
            'connections.close_all_success': 'All connections closed',
            'connections.close_all_failed': 'Batch close failed',
            'connections.empty': 'No active connections',
            'connections.loading': 'Loading...',
            'connections.search_placeholder': 'Search connections...',
            
            // Logs
            'logs.title': 'Logs',
            'logs.pause': 'Pause',
            'logs.resume': 'Resume',
            'logs.clear': 'Clear',
            'logs.ws_connected': 'WebSocket connected',
        }
    };

    function t(key) {
        const keys = key.split('.');
        let value = translations[currentLang];
        for (const k of keys) {
            value = value?.[k];
            if (!value) break;
        }
        return value || key;
    }

    function setLanguage(lang) {
        if (currentLang === lang) return;
        currentLang = lang;
        localStorage.setItem('lang', lang);
        console.log('[i18n] 语言切换到:', lang);
        window.dispatchEvent(new CustomEvent('languageChanged', { detail: { lang } }));
    }

    function getLanguage() {
        return currentLang;
    }

    return {
        t,
        setLanguage,
        getLanguage,
        translations
    };
})();
