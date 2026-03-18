// Simple SPA Router
const Router = {
    routes: {},
    currentPath: '',

    register(path, handler) {
        this.routes[path] = handler;
    },

    navigate(path) {
        window.history.pushState({}, '', path);
        this.resolve();
    },

    resolve() {
        const path = window.location.pathname || '/';
        this.currentPath = path;

        // Check if logged in
        if (!API.token && path !== '/login' && path !== '/register') {
            this.navigate('/login');
            return;
        }

        // Find matching route
        const handler = this.routes[path];
        if (handler) {
            handler();
        } else {
            // Default to dashboard
            if (API.token) {
                this.navigate('/');
            } else {
                this.navigate('/login');
            }
        }

        // Update active nav item
        document.querySelectorAll('.nav-item').forEach(el => {
            el.classList.toggle('active', el.dataset.path === path);
        });

        if (typeof window.syncShellState === 'function') {
            window.syncShellState(path);
        }
    },

    init() {
        window.addEventListener('popstate', () => this.resolve());

        document.addEventListener('click', (e) => {
            const navItem = e.target.closest('.nav-item[data-path]');
            if (navItem) {
                e.preventDefault();
                this.navigate(navItem.dataset.path);
            }
        });
    }
};
