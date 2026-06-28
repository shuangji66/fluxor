(function() {
    const pages = ['overview', 'proxies', 'rules', 'connections', 'logs', 'config', 'subscription'];
    let currentPage = 'overview';
    let mobileTabBarCreated = false;
    let mobileTopBarCreated = false;

    const moduleMap = {
        overview: 'Overview',
        proxies: 'Proxies',
        rules: 'Rules',
        connections: 'Connections',
        logs: 'Logs',
        config: 'Config',
        subscription: 'Subscription'
    };

    Object.values(moduleMap).forEach(name => {
        if (!window[name]) {
            window[name] = { init: function() {}, render: function() {} };
            console.warn('[App] 模块 ' + name + ' 未定义，使用占位');
        }
    });

    function ensureContainer(page) {
        const main = document.querySelector('.main-content');
        if (!main) return null;
        let container = document.getElementById(page + '-content');
        if (!container) {
            container = document.createElement('div');
            container.id = page + '-content';
            container.style.display = 'none';
            main.appendChild(container);
        }
        return container;
    }

    function createContainers() {
        pages.forEach(p => ensureContainer(p));
    }

    function showPage(page) {
        console.log('[App] 切换到页面:', page);
        pages.forEach(p => {
            const el = document.getElementById(p + '-content');
            if (el) el.style.display = 'none';
        });
        const active = ensureContainer(page);
        if (active) active.style.display = 'block';

        document.querySelectorAll('.nav-item').forEach(nav => {
            nav.classList.toggle('active', nav.dataset.page === page);
        });
        document.querySelectorAll('.mobile-tab-item').forEach(item => {
            item.classList.toggle('active', item.dataset.page === page);
        });

        const module = window[moduleMap[page]];
        try {
            if (typeof module.init === 'function') module.init();
            else if (typeof module.render === 'function') module.render();
        } catch (e) {
            console.error('[App] 模块 ' + page + ' 初始化错误:', e);
        }

        // 更新页面标题
        updateTitle();
    }

    // 新增：更新浏览器标签页标题
    function updateTitle() {
        const pageKey = 'nav.' + currentPage;
        const pageName = (window.i18n && window.i18n.t) ? window.i18n.t(pageKey) : currentPage;
        document.title = 'Fluxor - ' + pageName;
    }

    function updateSidebarLabels() {
        const t = window.i18n ? window.i18n.t : (key) => key;
        document.querySelectorAll('.nav-item').forEach(item => {
            const page = item.dataset.page;
            if (page) {
                const labelSpan = item.querySelector('.nav-label');
                if (labelSpan) labelSpan.textContent = t('nav.' + page);
            }
        });
        document.querySelectorAll('.mobile-tab-item').forEach(item => {
            const page = item.dataset.page;
            if (page) {
                const labelSpan = item.querySelector('.tab-label');
                if (labelSpan) labelSpan.textContent = t('nav.' + page);
            }
        });
        const langToggle = document.getElementById('langToggle');
        if (langToggle) {
            const currentLang = window.i18n ? window.i18n.getLanguage() : 'zh';
            const span = langToggle.querySelector('#currentLang');
            if (span) span.textContent = currentLang === 'zh' ? '简' : 'EN';
        }
        const mobileLangBtn = document.getElementById('mobileLangToggle');
        if (mobileLangBtn) {
            const currentLang = window.i18n ? window.i18n.getLanguage() : 'zh';
            const span = mobileLangBtn.querySelector('#mobileCurrentLang');
            if (span) span.textContent = currentLang === 'zh' ? '简' : 'EN';
        }
        // 语言变化时，标题也需要更新
        updateTitle();
    }

    function initNavigation() {
        document.querySelectorAll('.nav-item').forEach(nav => {
            nav.addEventListener('click', (e) => {
                e.preventDefault();
                const page = nav.dataset.page;
                if (page) {
                    currentPage = page;
                    showPage(page);
                }
            });
        });
        updateSidebarLabels();
    }

    function initLanguage() {
        const langToggle = document.getElementById('langToggle');
        if (langToggle && window.i18n) {
            langToggle.addEventListener('click', () => {
                const newLang = window.i18n.getLanguage() === 'zh' ? 'en' : 'zh';
                window.i18n.setLanguage(newLang);
                window.dispatchEvent(new CustomEvent('languageChanged', { detail: { lang: newLang } }));
                const mod = window[moduleMap[currentPage]];
                if (mod && typeof mod.destroy === 'function') mod.destroy();
                showPage(currentPage);
            });
        }
        window.addEventListener('languageChanged', () => {
            updateSidebarLabels(); // 内部已调用 updateTitle
        });
    }

    // ===== 侧边栏 =====
    function initSidebar() {
        const sidebar = document.getElementById('sidebar');
        const toggleBtn = document.getElementById('sidebarToggle');
        const overlay = document.getElementById('sidebarOverlay');
        if (!sidebar) return;

        if (toggleBtn) {
            toggleBtn.innerHTML = `
                <svg class="icon-hamburger" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <line x1="3" y1="6" x2="21" y2="6"></line>
                    <line x1="3" y1="12" x2="21" y2="12"></line>
                    <line x1="3" y1="18" x2="21" y2="18"></line>
                </svg>
                <svg class="icon-close" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="display:none;">
                    <line x1="18" y1="6" x2="6" y2="18"></line>
                    <line x1="6" y1="6" x2="18" y2="18"></line>
                </svg>
            `;
            const updateToggleIcon = () => {
                const isCollapsed = sidebar.classList.contains('collapsed');
                const hamburger = toggleBtn.querySelector('.icon-hamburger');
                const closeIcon = toggleBtn.querySelector('.icon-close');
                if (hamburger && closeIcon) {
                    hamburger.style.display = isCollapsed ? 'block' : 'none';
                    closeIcon.style.display = isCollapsed ? 'none' : 'block';
                }
            };
            toggleBtn._updateIcon = updateToggleIcon;
        }

        const isMobile = () => window.innerWidth <= 768;

        if (!isMobile() && localStorage.getItem('fluxor-sidebar-collapsed') === 'true') {
            sidebar.classList.add('collapsed');
        }

        function toggleSidebar() {
            if (isMobile()) {
                sidebar.classList.toggle('open');
                if (overlay) overlay.classList.toggle('active', sidebar.classList.contains('open'));
            } else {
                sidebar.classList.toggle('collapsed');
                localStorage.setItem('fluxor-sidebar-collapsed', sidebar.classList.contains('collapsed'));
                if (toggleBtn && toggleBtn._updateIcon) toggleBtn._updateIcon();
            }
        }

        function closeMobileSidebar() {
            sidebar.classList.remove('open');
            if (overlay) overlay.classList.remove('active');
        }

        if (toggleBtn) {
            toggleBtn.addEventListener('click', toggleSidebar);
            if (toggleBtn._updateIcon) toggleBtn._updateIcon();
        }
        if (overlay) overlay.addEventListener('click', closeMobileSidebar);

        let prevMobile = isMobile();
        window.addEventListener('resize', () => {
            const nowMobile = isMobile();
            if (prevMobile !== nowMobile) {
                sidebar.classList.remove('open', 'collapsed');
                if (overlay) overlay.classList.remove('active');
                if (!nowMobile && localStorage.getItem('fluxor-sidebar-collapsed') === 'true') {
                    sidebar.classList.add('collapsed');
                }
                prevMobile = nowMobile;
                updateMobileTabBarVisibility();
                updateMobileTopBarVisibility();
                if (toggleBtn && toggleBtn._updateIcon) toggleBtn._updateIcon();
            }
        });

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && isMobile() && sidebar.classList.contains('open')) {
                closeMobileSidebar();
            }
        });
    }

    // ===== 移动端底部导航 =====
    function updateMobileTabBarVisibility() {
        const isMobile = window.innerWidth <= 768;
        const tabBar = document.querySelector('.mobile-tabbar');
        if (tabBar) {
            tabBar.style.display = isMobile ? 'flex' : 'none';
        }
    }

    function initMobileTabBar() {
        if (mobileTabBarCreated) return;
        const tabBar = document.createElement('div');
        tabBar.className = 'mobile-tabbar';
        tabBar.style.display = 'none';
        const items = [
            { page: 'overview', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>', labelKey: 'nav.overview' },
            { page: 'proxies', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>', labelKey: 'nav.proxies' },
            { page: 'subscription', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="4" width="20" height="16" rx="2"/><polyline points="2 8 12 14 22 8"/></svg>', labelKey: 'nav.subscription' },
            { page: 'rules', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>', labelKey: 'nav.rules' },
            { page: 'connections', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>', labelKey: 'nav.connections' },
            { page: 'logs', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>', labelKey: 'nav.logs' },
            { page: 'config', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>', labelKey: 'nav.config' }
        ];

        items.forEach(item => {
            const btn = document.createElement('button');
            btn.className = 'mobile-tab-item';
            btn.dataset.page = item.page;
            const label = (window.i18n && window.i18n.t) ? window.i18n.t(item.labelKey) : item.labelKey.split('.')[1];
            btn.innerHTML = item.icon + '<span class="tab-label">' + label + '</span>';
            btn.addEventListener('click', () => {
                currentPage = item.page;
                showPage(item.page);
            });
            tabBar.appendChild(btn);
        });

        document.body.appendChild(tabBar);
        mobileTabBarCreated = true;
        updateMobileTabBarVisibility();
    }

    // ===== 移动端顶栏 =====
    function updateMobileTopBarVisibility() {
        const isMobile = window.innerWidth <= 768;
        const topbar = document.getElementById('mobile-topbar');
        if (topbar) {
            topbar.style.display = isMobile ? 'flex' : 'none';
        }
    }

    function initMobileTopBar() {
        if (mobileTopBarCreated) return;

        const topbar = document.createElement('div');
        topbar.id = 'mobile-topbar';
        topbar.innerHTML = `
            <span class="topbar-title" id="mobileTitle">Fluxor</span>
            <div class="topbar-actions">
                <button id="mobileLangToggle" aria-label="Switch Language">
                    <span id="mobileCurrentLang">简</span>
                </button>
                <button id="mobileThemeToggle" aria-label="Toggle Theme">
                    <svg class="icon-moon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
                    <svg class="icon-sun" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="display:none;"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                    <svg class="icon-auto" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="display:none;">
                        <circle cx="12" cy="12" r="10"/>
                        <text x="12" y="16" font-size="14" text-anchor="middle" fill="currentColor" stroke="none">A</text>
                    </svg>
                </button>
            </div>
        `;

        document.body.prepend(topbar);
        mobileTopBarCreated = true;

        const title = document.getElementById('mobileTitle');
        if (title) {
            title.addEventListener('click', () => {
                if (currentPage !== 'overview') {
                    currentPage = 'overview';
                    showPage('overview');
                }
            });
        }

        const mobileLangBtn = document.getElementById('mobileLangToggle');
        if (mobileLangBtn && window.i18n) {
            mobileLangBtn.addEventListener('click', () => {
                const newLang = window.i18n.getLanguage() === 'zh' ? 'en' : 'zh';
                window.i18n.setLanguage(newLang);
                window.dispatchEvent(new CustomEvent('languageChanged', { detail: { lang: newLang } }));
                const mod = window[moduleMap[currentPage]];
                if (mod && typeof mod.destroy === 'function') mod.destroy();
                showPage(currentPage);
            });
        }

        const mobileThemeBtn = document.getElementById('mobileThemeToggle');
        if (mobileThemeBtn && window.themeManager) {
            mobileThemeBtn.addEventListener('click', () => {
                const current = window.themeManager.getTheme();
                const cycle = { light: 'dark', dark: 'system', system: 'light' };
                window.themeManager.setTheme(cycle[current]);
            });
        }

        updateMobileTopBarVisibility();
        updateSidebarLabels();
    }

    // ========== 主题管理 ==========
    function initTheme() {
        const themeToggle = document.getElementById('themeToggle');

        let systemMediaQuery = null;
        let systemListener = null;

        function getSystemTheme() {
            return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
        }

        function setTheme(theme) {
            if (systemMediaQuery && systemListener) {
                systemMediaQuery.removeEventListener('change', systemListener);
                systemMediaQuery = null;
                systemListener = null;
            }

            localStorage.setItem('fluxor-theme', theme);

            let effectiveTheme = theme;
            if (theme === 'system') {
                effectiveTheme = getSystemTheme();
                systemMediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
                systemListener = function(e) {
                    document.documentElement.setAttribute('data-theme', e.matches ? 'dark' : 'light');
                };
                systemMediaQuery.addEventListener('change', systemListener);
            }

            document.documentElement.setAttribute('data-theme', effectiveTheme);

            document.querySelectorAll('.icon-moon').forEach(el => el.style.display = theme === 'dark' ? 'block' : 'none');
            document.querySelectorAll('.icon-sun').forEach(el => el.style.display = theme === 'light' ? 'block' : 'none');
            document.querySelectorAll('.icon-auto').forEach(el => el.style.display = theme === 'system' ? 'block' : 'none');

            window.dispatchEvent(new CustomEvent('themeChanged', { detail: { theme } }));
        }

        function getTheme() {
            return localStorage.getItem('fluxor-theme') || 'system';
        }

        window.themeManager = { setTheme, getTheme };

        const saved = getTheme();
        setTheme(saved);

        if (themeToggle) {
            themeToggle.addEventListener('click', () => {
                const current = getTheme();
                const cycle = { light: 'dark', dark: 'system', system: 'light' };
                setTheme(cycle[current]);
            });
        }
    }

    function init() {
        console.log('[App] 初始化开始');
        createContainers();
        initNavigation();
        initSidebar();
        initTheme();
        initLanguage();
        initMobileTabBar();
        initMobileTopBar();
        updateMobileTabBarVisibility();
        updateMobileTopBarVisibility();
        showPage('overview');
        console.log('[App] 初始化完成');
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();