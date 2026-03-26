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
    login(username, password) {
        return this.request('POST', '/api/auth/login', { username, password: normalizePasswordValue(password) });
    },
    register(username, password) {
        return this.request('POST', '/api/auth/register', { username, password: normalizePasswordValue(password) });
    },

    // Dashboard
    getDashboard() { return this.request('GET', '/api/dashboard'); },

    // Nodes
    getNodes() { return this.request('GET', '/api/nodes'); },
    createNode(nodeOrName, group_name, owner_user_id) {
        if (typeof nodeOrName === 'object' && nodeOrName !== null) {
            return this.request('POST', '/api/nodes', nodeOrName);
        }
        return this.request('POST', '/api/nodes', { name: nodeOrName, group_name, owner_user_id });
    },
    getNode(id) { return this.request('GET', `/api/nodes/${id}`); },
    deleteNode(id) { return this.request('DELETE', `/api/nodes/${id}`); },
    getNodeGroups() { return this.request('GET', '/api/node-groups'); },
    createNodeGroup(name, description) { return this.request('POST', '/api/node-groups', { name, description }); },
    deleteNodeGroup(id) { return this.request('DELETE', `/api/node-groups/${id}`); },

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
    resetTraffic(id) { return this.request('POST', `/api/rules/${id}/reset-traffic`); },
    testLatency(id) { return this.request('POST', `/api/rules/${id}/test-latency`); },
    getChainLatency(id) { return this.request('GET', `/api/rules/${id}/chain-latency`); },

    // Traffic
    getTraffic(ruleId, hours) { return this.request('GET', `/api/traffic/${ruleId}?hours=${hours || 24}`); },
    getAggregateTraffic(hours) { return this.request('GET', `/api/traffic/aggregate?hours=${hours || 24}`); },
    getEvents(limit) { return this.request('GET', `/api/events?limit=${limit || 12}`); },

    // Users
    getUsers() { return this.request('GET', '/api/users'); },
    createUser(userOrUsername, password) {
        if (typeof userOrUsername === 'object' && userOrUsername !== null) {
            const payload = { ...userOrUsername };
            if (payload.password) {
                payload.password = normalizePasswordValue(payload.password);
            }
            return this.request('POST', '/api/users', payload);
        }
        return this.request('POST', '/api/users', {
            username: userOrUsername,
            password: normalizePasswordValue(password),
        });
    },
    deleteUser(id) { return this.request('DELETE', `/api/users/${id}`); },
    changePassword(old_password, new_password) {
        return this.request('POST', '/api/user/password', {
            old_password: normalizePasswordValue(old_password),
            new_password: normalizePasswordValue(new_password)
        });
    },
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
        const iconSpan = document.createElement('span');
        iconSpan.textContent = icons[type] || 'ℹ';
        const msgSpan = document.createElement('span');
        msgSpan.textContent = message;
        toast.appendChild(iconSpan);
        toast.appendChild(msgSpan);
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

function isManagerRole(role) {
    return role === 'admin' || role === 'reseller';
}

function isAdminRole(role) {
    return role === 'admin';
}

function buildUserMap(users) {
    return (users || []).reduce((acc, user) => {
        acc[user.id] = user;
        return acc;
    }, {});
}

function resolveUserLabel(userId, userMap, fallbackUser) {
    if (userId && userMap && userMap[userId] && userMap[userId].username) {
        return userMap[userId].username;
    }
    if (fallbackUser && fallbackUser.id === userId) {
        return fallbackUser.username;
    }
    return userId ? `#${userId}` : '-';
}

function formatNullableDateTime(value) {
    if (!value) return 'Never';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'Never';
    return date.toLocaleString();
}

function formatAccountTrafficQuota(quota, used) {
    if (!quota || quota <= 0) {
        return `${formatBytes(used || 0)} / Unlimited`;
    }
    return formatTrafficWithLimit(used || 0, quota);
}

function formatBandwidthLimit(limit) {
    return limit && limit > 0 ? `${limit} KB/s` : 'Unlimited';
}

// Utility: format bytes
function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 2 : 0) + ' ' + units[i];
}

function formatNodeMemory(node) {
    const usedBytes = Number(node?.mem_usage || 0) * 1024 * 1024;
    const totalBytes = Number(node?.mem_total || 0) * 1024 * 1024;

    if (totalBytes > 0) {
        return `${formatBytes(usedBytes)} / ${formatBytes(totalBytes)}`;
    }

    return formatBytes(usedBytes);
}

function normalizePasswordValue(value) {
    return (value || '').replace(/\s+/g, '');
}

function bindPasswordSanitizers(root = document) {
    const inputs = root.querySelectorAll('input[type="password"]');

    inputs.forEach(input => {
        if (input.dataset.passwordSanitized === '1') {
            return;
        }

        const apply = () => {
            const normalized = normalizePasswordValue(input.value);
            if (input.value !== normalized) {
                input.value = normalized;
            }
        };

        input.dataset.passwordSanitized = '1';
        input.addEventListener('input', apply);
        input.addEventListener('change', apply);
        input.addEventListener('paste', () => setTimeout(apply, 0));
        apply();
    });
}

// Utility: format traffic with limit
function formatTrafficWithLimit(used, limit) {
    if (!limit || limit <= 0) return formatBytes(used);
    const pct = Math.min(100, (used / limit) * 100);
    const color = pct >= 90 ? 'var(--color-danger)' : pct >= 70 ? 'var(--color-warning)' : 'var(--color-success)';
    return `<span style="color:${color};">${formatBytes(used)}</span> / ${formatBytes(limit)} <span style="color:${color};font-size:0.75rem;">(${pct.toFixed(0)}%)</span>`;
}

// Utility: parse traffic limit from input (supports GB/MB/KB suffix)
function parseTrafficLimit(value) {
    if (!value || value === '0') return 0;
    const str = String(value).trim().toUpperCase();
    const num = parseFloat(str);
    if (isNaN(num)) return 0;
    if (str.endsWith('TB')) return Math.round(num * 1024 * 1024 * 1024 * 1024);
    if (str.endsWith('GB')) return Math.round(num * 1024 * 1024 * 1024);
    if (str.endsWith('MB')) return Math.round(num * 1024 * 1024);
    if (str.endsWith('KB')) return Math.round(num * 1024);
    // Default to GB if just a number
    return Math.round(num * 1024 * 1024 * 1024);
}

// Utility: format traffic limit for input display
function formatTrafficLimitInput(bytes) {
    if (!bytes || bytes <= 0) return '';
    if (bytes >= 1024 * 1024 * 1024 * 1024) return (bytes / (1024 * 1024 * 1024 * 1024)).toFixed(1) + 'TB';
    if (bytes >= 1024 * 1024 * 1024) return (bytes / (1024 * 1024 * 1024)).toFixed(1) + 'GB';
    if (bytes >= 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(0) + 'MB';
    return (bytes / 1024).toFixed(0) + 'KB';
}

// Utility: format latency
function formatLatency(ms) {
    if (ms === undefined || ms === null || ms < 0) return '<span style="color:var(--color-muted);">N/A</span>';
    if (ms < 1) return '<span style="color:var(--color-success);">&lt;1ms</span>';
    const color = ms < 50 ? 'var(--color-success)' : ms < 150 ? 'var(--color-warning, #e6a23c)' : 'var(--color-danger)';
    return `<span style="color:${color};">${ms.toFixed(1)}ms</span>`;
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
