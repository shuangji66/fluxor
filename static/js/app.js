(function() {
    const pages = ['overview', 'proxies', 'rules', 'connections', 'logs', 'config', 'subscription'];
    let currentPage = 'overview';
    let mobileTabBarCreated = false;

    const moduleMap = {
        overview: 'Overview',
        proxies: 'Proxies',
        rules: 'Rules',
        connections: 'Connections',
        logs: 'Logs',
        config: 'Config',
        subscription: 'Subscription'
    };

    // 确保模块存在
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
    }

    function initSidebar() {
        const sidebar = document.getElementById('sidebar');
        const toggleBtn = document.getElementById('sidebarToggle');
        const overlay = document.getElementById('sidebarOverlay');
        if (!sidebar) return;

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
            }
        }

        function closeMobileSidebar() {
            sidebar.classList.remove('open');
            if (overlay) overlay.classList.remove('active');
        }

        if (toggleBtn) toggleBtn.addEventListener('click', toggleSidebar);
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
            }
        });

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && isMobile() && sidebar.classList.contains('open')) {
                closeMobileSidebar();
            }
        });
    }

    function updateMobileTabBarVisibility() {
        const isMobile = window.innerWidth <= 768;
        const tabBar = document.querySelector('.mobile-tabbar');
        if (isMobile) {
            if (!tabBar) initMobileTabBar();
            else tabBar.style.display = 'flex';
        } else {
            if (tabBar) tabBar.style.display = 'none';
        }
    }

    function initMobileTabBar() {
        if (mobileTabBarCreated) return;
        if (window.innerWidth > 768) return;

        const tabBar = document.createElement('div');
        tabBar.className = 'mobile-tabbar';
        const items = [
            { page: 'overview', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>', labelKey: 'nav.overview' },
            { page: 'proxies', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>', labelKey: 'nav.proxies' },
            { page: 'rules', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>', labelKey: 'nav.rules' },
            { page: 'connections', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>', labelKey: 'nav.connections' },
            { page: 'logs', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>', labelKey: 'nav.logs' },
            { page: 'config', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>', labelKey: 'nav.config' },
            { page: 'subscription', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 4v16h16V4H4zm2 2h12v12H6V6zm10 4h-2v2h2v-2zm-4 0h-2v2h2v-2zm-4 0H6v2h2v-2z"/><path d="M8 14h8v2H8z"/></svg>', labelKey: 'nav.subscription' }
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

    function initTheme() {
        const themeToggle = document.getElementById('themeToggle');
        const moonIcon = document.querySelector('.icon-moon');
        const sunIcon = document.querySelector('.icon-sun');
        function setTheme(theme) {
            document.documentElement.setAttribute('data-theme', theme);
            localStorage.setItem('fluxor-theme', theme);
            if (moonIcon && sunIcon) {
                moonIcon.style.display = theme === 'dark' ? 'block' : 'none';
                sunIcon.style.display = theme === 'dark' ? 'none' : 'block';
            }
        }
        const saved = localStorage.getItem('fluxor-theme');
        setTheme(saved === 'light' ? 'light' : 'dark');
        if (themeToggle) {
            themeToggle.addEventListener('click', () => {
                const current = document.documentElement.getAttribute('data-theme');
                setTheme(current === 'dark' ? 'light' : 'dark');
            });
        }
    }

    function initLanguage() {
        const langToggle = document.getElementById('langToggle');
        if (langToggle && window.i18n) {
            langToggle.addEventListener('click', () => {
                const newLang = window.i18n.getLanguage() === 'zh' ? 'en' : 'zh';
                window.i18n.setLanguage(newLang);
                showPage(currentPage);
                if (window.innerWidth <= 768 && mobileTabBarCreated) {
                    document.querySelectorAll('.mobile-tab-item').forEach(item => {
                        const key = item.getAttribute('data-page');
                        const label = window.i18n.t('nav.' + key);
                        const span = item.querySelector('.tab-label');
                        if (span) span.textContent = label;
                    });
                }
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
        updateMobileTabBarVisibility();
        showPage('overview');
        console.log('[App] 初始化完成');
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();