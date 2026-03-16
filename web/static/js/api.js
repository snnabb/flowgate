// FlowGate API Client
const API = {
    token: localStorage.getItem('fg_token'),
    baseURL: '',

    setToken(token) {
        this.token = token;
        localStorage.setItem('fg_token', token);
    },

    clearToken() {
        this.token = null;
        localStorage.removeItem('fg_token');
        localStorage.removeItem('fg_user');
    },

    getUser() {
        try { return JSON.parse(localStorage.getItem('fg_user')); }
        catch { return null; }
    },

    setUser(user) {
        localStorage.setItem('fg_user', JSON.stringify(user));
    },

    async request(method, path, body) {
        const headers = { 'Content-Type': 'application/json' };
        if (this.token) headers['Authorization'] = `Bearer ${this.token}`;

        const opts = { method, headers };
        if (body) opts.body = JSON.stringify(body);

        const res = await fetch(this.baseURL + path, opts);
        const data = await res.json();

        if (res.status === 401) {
            this.clearToken();
            Router.navigate('/login');
            throw new Error('Unauthorized');
        }

        if (!res.ok) throw new Error(data.error || 'Request failed');
        return data;
    },

    // Auth
    checkSetup() { return this.request('GET', '/api/auth/setup'); },
    login(username, password) { return this.request('POST', '/api/auth/login', { username, password }); },
    register(username, password) { return this.request('POST', '/api/auth/register', { username, password }); },

    // Dashboard
    getDashboard() { return this.request('GET', '/api/dashboard'); },

    // Nodes
    getNodes() { return this.request('GET', '/api/nodes'); },
    createNode(name, group_name) { return this.request('POST', '/api/nodes', { name, group_name }); },
    getNode(id) { return this.request('GET', `/api/nodes/${id}`); },
    deleteNode(id) { return this.request('DELETE', `/api/nodes/${id}`); },

    // Rules
    getRules(nodeId) {
        const q = nodeId ? `?node_id=${nodeId}` : '';
        return this.request('GET', `/api/rules${q}`);
    },
    createRule(rule) { return this.request('POST', '/api/rules', rule); },
    getRule(id) { return this.request('GET', `/api/rules/${id}`); },
    updateRule(id, rule) { return this.request('PUT', `/api/rules/${id}`, rule); },
    deleteRule(id) { return this.request('DELETE', `/api/rules/${id}`); },
    toggleRule(id) { return this.request('POST', `/api/rules/${id}/toggle`); },

    // Traffic
    getTraffic(ruleId, hours) { return this.request('GET', `/api/traffic/${ruleId}?hours=${hours || 24}`); },

    // Users
    getUsers() { return this.request('GET', '/api/users'); },
    deleteUser(id) { return this.request('DELETE', `/api/users/${id}`); },
    changePassword(old_password, new_password) { return this.request('POST', '/api/user/password', { old_password, new_password }); },
};

// Toast notification helper
const Toast = {
    container: null,

    init() {
        this.container = document.createElement('div');
        this.container.className = 'toast-container';
        document.body.appendChild(this.container);
    },

    show(message, type = 'info') {
        if (!this.container) this.init();
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        const icons = { success: '✓', error: '✕', info: 'ℹ' };
        toast.innerHTML = `<span>${icons[type] || 'ℹ'}</span><span>${message}</span>`;
        this.container.appendChild(toast);
        setTimeout(() => {
            toast.style.opacity = '0';
            toast.style.transform = 'translateX(100%)';
            toast.style.transition = 'all 0.3s ease';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    },

    success(msg) { this.show(msg, 'success'); },
    error(msg) { this.show(msg, 'error'); },
    info(msg) { this.show(msg, 'info'); },
};

// Utility: format bytes
function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 2 : 0) + ' ' + units[i];
}

// Utility: format speed
function formatSpeed(bytesPerSec) {
    if (!bytesPerSec) return '0 B/s';
    return formatBytes(bytesPerSec) + '/s';
}

// Utility: copy to clipboard
async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        Toast.success('已复制到剪贴板');
    } catch {
        const ta = document.createElement('textarea');
        ta.value = text;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        ta.remove();
        Toast.success('已复制到剪贴板');
    }
}
