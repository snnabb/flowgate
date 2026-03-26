// FlowGate Main App
(function() {
    'use strict';

    window.appNeedsSetup = false;

    const icons = {
        menu: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="3" y1="6" x2="21" y2="6"></line><line x1="3" y1="12" x2="21" y2="12"></line><line x1="3" y1="18" x2="21" y2="18"></line></svg>',
        close: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>',
        dashboard: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"></rect><rect x="14" y="3" width="7" height="7"></rect><rect x="14" y="14" width="7" height="7"></rect><rect x="3" y="14" width="7" height="7"></rect></svg>',
        nodes: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>',
        rules: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 3 21 3 21 8"></polyline><line x1="4" y1="20" x2="21" y2="3"></line><polyline points="21 16 21 21 16 21"></polyline><line x1="15" y1="15" x2="21" y2="21"></line><line x1="4" y1="4" x2="9" y2="9"></line></svg>',
        stats: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"></line><line x1="12" y1="20" x2="12" y2="4"></line><line x1="6" y1="20" x2="6" y2="14"></line></svg>',
        users: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle><path d="M23 21v-2a4 4 0 0 0-3-3.87"></path><path d="M16 3.13a4 4 0 0 1 0 7.75"></path></svg>',
        logout: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path><polyline points="16 17 21 12 16 7"></polyline><line x1="21" y1="12" x2="9" y2="12"></line></svg>',
    };

    const shellTitles = {
        '/': '仪表盘',
        '/nodes': '节点管理',
        '/rules': '转发规则',
        '/stats': '流量统计',
        '/users': '用户管理'
    };

    async function initApp() {
        Router.init();

        Router.register('/', renderDashboard);
        Router.register('/nodes', renderNodes);
        Router.register('/rules', renderRules);
        Router.register('/stats', renderStats);
        Router.register('/users', renderUsers);
        Router.register('/login', renderLogin);
        Router.register('/register', renderRegister);

        if (!API.token) {
            try {
                const setup = await API.checkSetup();
                window.appNeedsSetup = !!setup.needs_setup;
                Router.navigate(window.appNeedsSetup ? '/register' : '/login');
            } catch {
                window.appNeedsSetup = false;
                Router.navigate('/login');
            }
        } else {
            renderLayout();
            Router.resolve();
            connectPanelWS();
        }

        setTimeout(() => {
            const loadingScreen = document.getElementById('loading-screen');
            if (!loadingScreen) return;

            loadingScreen.classList.add('fade-out');
            setTimeout(() => loadingScreen.remove(), 500);
        }, 300);
    }

    function renderLayout() {
        const user = API.getUser();
        const app = document.getElementById('app');

        app.innerHTML = `
            <div class="layout">
                <div class="sidebar-backdrop" id="sidebar-backdrop" onclick="closeSidebar()"></div>
                <aside class="sidebar" id="sidebar">
                    <div class="sidebar-brand">
                        <div class="sidebar-brand-copy">
                            <h1>FlowGate</h1>
                            <span>端口转发管理面板</span>
                        </div>
                        <button class="sidebar-close" type="button" onclick="closeSidebar()" aria-label="关闭导航">
                            ${icons.close}
                        </button>
                    </div>
                    <nav class="sidebar-nav">
                        <div class="nav-item" data-path="/">${icons.dashboard}<span>仪表盘</span></div>
                        <div class="nav-item" data-path="/nodes">${icons.nodes}<span>节点管理</span></div>
                        <div class="nav-item" data-path="/rules">${icons.rules}<span>转发规则</span></div>
                        <div class="nav-item" data-path="/stats">${icons.stats}<span>流量统计</span></div>
                        <div class="nav-item" data-path="/users">${icons.users}<span>用户管理</span></div>
                    </nav>
                    <div class="sidebar-footer">
                        <div class="sidebar-account">
                            <div class="sidebar-user-card">
                                <div class="avatar">${user ? user.username[0].toUpperCase() : '?'}</div>
                                <div class="user-info">
                                    <div class="user-name">${user ? escHTML(user.username) : '未知用户'}</div>
                                    <div class="user-role">${user ? user.role : ''}</div>
                                </div>
                            </div>
                            <button class="sidebar-logout-btn" type="button" onclick="handleLogout()">
                                ${icons.logout}
                                <span>退出登录</span>
                            </button>
                        </div>
                    </div>
                </aside>

                <div class="mobile-topbar">
                    <div class="mobile-topbar-title" id="mobile-topbar-title">${getShellTitle(Router.currentPath || window.location.pathname || '/')}</div>
                    <button class="mobile-topbar-btn" type="button" onclick="handleLogout()" aria-label="退出登录">
                        ${icons.logout}
                    </button>
                </div>

                <main class="main-content" id="page-content"></main>

                <nav class="bottom-nav" id="bottom-nav">
                    <button class="bottom-nav-item" data-path="/" onclick="Router.navigate('/')">
                        ${icons.dashboard}
                        <span class="bottom-nav-label">概览</span>
                    </button>
                    <button class="bottom-nav-item" data-path="/nodes" onclick="Router.navigate('/nodes')">
                        ${icons.nodes}
                        <span class="bottom-nav-label">节点</span>
                    </button>
                    <button class="bottom-nav-item" data-path="/rules" onclick="Router.navigate('/rules')">
                        ${icons.rules}
                        <span class="bottom-nav-label">规则</span>
                    </button>
                    <button class="bottom-nav-item" data-path="/stats" onclick="Router.navigate('/stats')">
                        ${icons.stats}
                        <span class="bottom-nav-label">统计</span>
                    </button>
                    <button class="bottom-nav-item" data-path="/users" onclick="Router.navigate('/users')">
                        ${icons.users}
                        <span class="bottom-nav-label">用户</span>
                    </button>
                </nav>
            </div>

            <div class="modal-overlay" id="modal-overlay" onclick="handleModalOverlayClick(event)">
                <div class="modal" id="modal">
                    <div class="modal-header">
                        <h3 id="modal-title"></h3>
                        <button class="modal-close" onclick="closeModal()">x</button>
                    </div>
                    <div class="modal-body" id="modal-body"></div>
                    <div class="modal-footer" id="modal-footer"></div>
                </div>
            </div>
        `;
    }

    function getShellTitle(path) {
        return shellTitles[path] || 'FlowGate';
    }

    function setSidebarOpen(open) {
        const sidebar = document.getElementById('sidebar');
        const backdrop = document.getElementById('sidebar-backdrop');
        if (!sidebar || !backdrop) return;

        sidebar.classList.toggle('open', open);
        backdrop.classList.toggle('active', open);
        document.body.classList.toggle('sidebar-open', open);
    }

    function getLoginFooterHTML() {
        if (window.appNeedsSetup) {
            return '还没有账号？<a href="/register" onclick="event.preventDefault();Router.navigate(\'/register\')">注册管理员</a>';
        }

        return '系统已初始化，请使用已有账号登录';
    }

    window.toggleSidebar = function(force) {
        const sidebar = document.getElementById('sidebar');
        if (!sidebar) return;

        const open = typeof force === 'boolean' ? force : !sidebar.classList.contains('open');
        setSidebarOpen(open);
    };

    window.closeSidebar = function() {
        setSidebarOpen(false);
    };

    window.syncShellState = function(path) {
        const title = document.getElementById('mobile-topbar-title');
        if (title) {
            title.textContent = getShellTitle(path);
        }

        // Update bottom nav active state
        const bottomNav = document.getElementById('bottom-nav');
        if (bottomNav) {
            bottomNav.querySelectorAll('.bottom-nav-item').forEach(item => {
                item.classList.toggle('active', item.dataset.path === path);
            });
        }
    };

    window.renderLogin = function() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="login-page">
                <div class="login-card">
                    <h1>FlowGate</h1>
                    <p class="subtitle">轻量级端口转发管理面板</p>
                    <form id="login-form" onsubmit="return handleLogin(event)">
                        <div class="form-group">
                            <label>用户名</label>
                            <input type="text" class="form-input" id="login-username" placeholder="输入用户名" autofocus required>
                        </div>
                        <div class="form-group">
                            <label>密码</label>
                            <input type="password" class="form-input" id="login-password" placeholder="输入密码" required>
                        </div>
                        <button type="submit" class="btn btn-primary">登 录</button>
                    </form>
                    <div class="login-footer">${getLoginFooterHTML()}</div>
                </div>
            </div>
        `;
        bindPasswordSanitizers(app);
    };

    window.renderRegister = function() {
        if (!window.appNeedsSetup) {
            Router.navigate('/login');
            Toast.info('系统初始化已完成，请直接登录');
            return;
        }

        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="login-page">
                <div class="login-card">
                    <h1>FlowGate</h1>
                    <p class="subtitle">创建首个管理员账号</p>
                    <form id="register-form" onsubmit="return handleRegister(event)">
                        <div class="form-group">
                            <label>用户名</label>
                            <input type="text" class="form-input" id="reg-username" placeholder="设置用户名" autofocus required>
                        </div>
                        <div class="form-group">
                            <label>密码</label>
                            <input type="password" class="form-input" id="reg-password" placeholder="设置密码 (至少6位)" required minlength="6">
                        </div>
                        <div class="form-group">
                            <label>确认密码</label>
                            <input type="password" class="form-input" id="reg-confirm" placeholder="再次输入密码" required>
                        </div>
                        <button type="submit" class="btn btn-primary">注 册</button>
                    </form>
                    <div class="login-footer">
                        已有账号？<a href="/login" onclick="event.preventDefault();Router.navigate('/login')">登录</a>
                    </div>
                </div>
            </div>
        `;
        bindPasswordSanitizers(app);
    };

    window.handleLogin = async function(e) {
        e.preventDefault();
        const username = document.getElementById('login-username').value.trim();
        const password = normalizePasswordValue(document.getElementById('login-password').value);
        document.getElementById('login-password').value = password;

        try {
            const res = await API.login(username, password);
            API.setToken(res.token);
            API.setUser(res.user);
            renderLayout();
            Router.navigate('/');
            connectPanelWS();
            Toast.success(`欢迎回来, ${res.user.username}!`);
        } catch (err) {
            Toast.error('登录失败: ' + err.message);
        }
        return false;
    };

    window.handleRegister = async function(e) {
        e.preventDefault();
        const username = document.getElementById('reg-username').value.trim();
        const password = normalizePasswordValue(document.getElementById('reg-password').value);
        const confirm = normalizePasswordValue(document.getElementById('reg-confirm').value);
        document.getElementById('reg-password').value = password;
        document.getElementById('reg-confirm').value = confirm;

        if (password !== confirm) {
            Toast.error('两次密码不一致');
            return false;
        }
        if (password.length < 6) {
            Toast.error('密码至少6位');
            return false;
        }

        try {
            const res = await API.register(username, password);
            window.appNeedsSetup = false;
            API.setToken(res.token);
            API.setUser(res.user);
            renderLayout();
            Router.navigate('/');
            connectPanelWS();
            Toast.success('管理员账号创建成功');
        } catch (err) {
            Toast.error('注册失败: ' + err.message);
        }
        return false;
    };

    window.handleLogout = function() {
        closeSidebar();
        disconnectPanelWS();
        API.clearToken();
        Router.navigate('/login');
        Toast.info('已退出登录');
    };

    window.showModal = function(title, body, onConfirm, cancelText, confirmText) {
        document.getElementById('modal-title').textContent = title;
        document.getElementById('modal-body').innerHTML = body;

        let footerHTML = `<button class="btn btn-secondary" onclick="closeModal()">${cancelText || '取消'}</button>`;
        if (onConfirm) {
            footerHTML += `<button class="btn btn-primary" id="modal-confirm-btn">${confirmText || '确认'}</button>`;
        }
        document.getElementById('modal-footer').innerHTML = footerHTML;

        if (onConfirm) {
            document.getElementById('modal-confirm-btn').onclick = onConfirm;
        }

        document.getElementById('modal-overlay').classList.add('active');
    };

    window.closeModal = function() {
        document.getElementById('modal-overlay').classList.remove('active');
    };

    window.handleModalOverlayClick = function(e) {
        if (e.target.id === 'modal-overlay') closeModal();
    };

    let panelWS = null;
    let wsReconnectTimer = null;
    let refreshInterval = null;

    function connectPanelWS() {
        if (panelWS) { panelWS.close(); panelWS = null; }
        if (!API.token) return;

        const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
        const url = `${proto}://${window.location.host}/ws/panel?token=${API.token}`;
        panelWS = new WebSocket(url);

        panelWS.onopen = function() {
            // WebSocket connected, stop polling fallback
            if (refreshInterval) { clearInterval(refreshInterval); refreshInterval = null; }
            if (wsReconnectTimer) { clearTimeout(wsReconnectTimer); wsReconnectTimer = null; }
        };

        panelWS.onmessage = function(e) {
            try {
                const msg = JSON.parse(e.data);
                if (msg.type === 'reload') {
                    if (Router.currentPath === '/') loadDashboardData();
                } else if (msg.type === 'latency_result' && msg.results) {
                    handleLatencyResults(msg.results);
                }
            } catch (_) {}
        };

        panelWS.onclose = function() {
            panelWS = null;
            // Reconnect after 5s, start polling as fallback
            if (API.token) {
                wsReconnectTimer = setTimeout(connectPanelWS, 5000);
                startPollingFallback();
            }
        };

        panelWS.onerror = function() { panelWS.close(); };
    }

    function disconnectPanelWS() {
        if (wsReconnectTimer) { clearTimeout(wsReconnectTimer); wsReconnectTimer = null; }
        if (panelWS) { panelWS.close(); panelWS = null; }
        if (refreshInterval) { clearInterval(refreshInterval); refreshInterval = null; }
    }

    function startPollingFallback() {
        if (refreshInterval) return;
        refreshInterval = setInterval(() => {
            if (Router.currentPath === '/') {
                loadDashboardData();
            }
        }, 15000);
    }

    document.addEventListener('DOMContentLoaded', () => {
        initApp();

        document.addEventListener('keydown', event => {
            if (event.key === 'Escape') {
                closeSidebar();
            }
        });

        window.addEventListener('resize', () => {
            if (window.innerWidth > 768) {
                closeSidebar();
            }
        });
    });
})();
