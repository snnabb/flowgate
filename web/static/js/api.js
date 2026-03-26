// FlowGate API client and shared helpers.
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
        try {
            return JSON.parse(localStorage.getItem('fg_user'));
        } catch {
            return null;
        }
    },

    setUser(user) {
        localStorage.setItem('fg_user', JSON.stringify(user));
    },

    async request(method, path, body) {
        const headers = { 'Content-Type': 'application/json' };
        if (this.token) {
            headers.Authorization = `Bearer ${this.token}`;
        }

        const options = { method, headers };
        if (body !== undefined) {
            options.body = JSON.stringify(body);
        }

        const response = await fetch(this.baseURL + path, options);
        const text = await response.text();
        const data = text ? JSON.parse(text) : {};

        if (response.status === 401) {
            this.clearToken();
            Router.navigate('/login');
            throw new Error('Unauthorized');
        }

        if (!response.ok) {
            throw new Error(data.error || 'Request failed');
        }
        return data;
    },

    // Auth
    checkSetup() { return this.request('GET', '/api/auth/setup'); },
    login(username, password) {
        return this.request('POST', '/api/auth/login', {
            username,
            password: normalizePasswordValue(password),
        });
    },
    register(username, password) {
        return this.request('POST', '/api/auth/register', {
            username,
            password: normalizePasswordValue(password),
        });
    },

    // Dashboard
    getDashboard() { return this.request('GET', '/api/dashboard'); },

    // Nodes
    getNodes() { return this.request('GET', '/api/nodes'); },
    createNode(payload) { return this.request('POST', '/api/nodes', payload); },
    getNode(id) { return this.request('GET', `/api/nodes/${id}`); },
    deleteNode(id) { return this.request('DELETE', `/api/nodes/${id}`); },

    // Rules
    getRules(nodeId) {
        const query = nodeId ? `?node_id=${nodeId}` : '';
        return this.request('GET', `/api/rules${query}`);
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
    createUser(payload) {
        return this.request('POST', '/api/users', {
            ...payload,
            password: normalizePasswordValue(payload.password),
        });
    },
    updateUser(id, payload) { return this.request('PUT', `/api/users/${id}`, payload); },
    deleteUser(id) { return this.request('DELETE', `/api/users/${id}`); },
    getUserAccess(id) { return this.request('GET', `/api/users/${id}/access`); },
    replaceUserAccess(id, payload) { return this.request('PUT', `/api/users/${id}/access`, payload); },
    getSelfAccess() { return this.request('GET', '/api/user/access'); },
    changePassword(oldPassword, newPassword) {
        return this.request('POST', '/api/user/password', {
            old_password: normalizePasswordValue(oldPassword),
            new_password: normalizePasswordValue(newPassword),
        });
    },
};

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
        const icons = { success: 'OK', error: 'ERR', info: 'INFO' };
        toast.innerHTML = `<strong>${icons[type] || 'INFO'}</strong><span>${escHTML(message)}</span>`;
        this.container.appendChild(toast);
        setTimeout(() => {
            toast.style.opacity = '0';
            toast.style.transform = 'translateX(100%)';
            toast.style.transition = 'all 0.3s ease';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    },

    success(message) { this.show(message, 'success'); },
    error(message) { this.show(message, 'error'); },
    info(message) { this.show(message, 'info'); },
};

function isManagerRole(role) {
    return role === 'admin';
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

function escHTML(value) {
    const div = document.createElement('div');
    div.textContent = value == null ? '' : String(value);
    return div.innerHTML;
}

function formatBytes(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let value = bytes;
    let unitIndex = 0;
    while (value >= 1024 && unitIndex < units.length - 1) {
        value /= 1024;
        unitIndex++;
    }
    return `${value.toFixed(unitIndex === 0 ? 0 : 2)} ${units[unitIndex]}`;
}

function formatBytesShort(bytes) {
    if (!bytes || bytes <= 0) return '0';
    const units = ['B', 'K', 'M', 'G', 'T'];
    let value = bytes;
    let unitIndex = 0;
    while (value >= 1024 && unitIndex < units.length - 1) {
        value /= 1024;
        unitIndex++;
    }
    return `${value.toFixed(unitIndex === 0 ? 0 : 1)}${units[unitIndex]}`;
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
    root.querySelectorAll('input[type="password"]').forEach((input) => {
        if (input.dataset.passwordSanitized === '1') return;
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

function formatTrafficWithLimit(used, limit) {
    if (!limit || limit <= 0) {
        return formatBytes(used);
    }
    const pct = Math.min(100, (used / limit) * 100);
    const color = pct >= 90 ? 'var(--color-danger)' : pct >= 70 ? 'var(--color-warning)' : 'var(--color-success)';
    return `<span style="color:${color};">${formatBytes(used)}</span> / ${formatBytes(limit)} <span style="color:${color};font-size:0.75rem;">(${pct.toFixed(0)}%)</span>`;
}

function formatLatency(ms) {
    if (ms === undefined || ms === null || ms < 0) {
        return '<span style="color:var(--text-muted);">N/A</span>';
    }
    if (ms < 1) {
        return '<span style="color:var(--color-success);">&lt;1ms</span>';
    }
    const color = ms < 50 ? 'var(--color-success)' : ms < 150 ? 'var(--color-warning)' : 'var(--color-danger)';
    return `<span style="color:${color};">${ms.toFixed(1)}ms</span>`;
}

function formatNullableDateTime(value) {
    if (!value) return 'Never';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'Never';
    return date.toLocaleString();
}

function formatBandwidthLimit(limitKB) {
    return `${bandwidthKBToM(limitKB)} M`;
}

function bandwidthKBToM(limitKB) {
    if (!limitKB || limitKB <= 0) return 'Unlimited';
    return (Number(limitKB) / 1024).toFixed(Number(limitKB) % 1024 === 0 ? 0 : 1);
}

function parseBandwidthM(value) {
    if (value == null) return 0;
    const text = String(value).trim();
    if (!text) return 0;
    const numeric = parseFloat(text.replace(/[^\d.]/g, ''));
    if (!Number.isFinite(numeric) || numeric <= 0) return 0;
    return Math.round(numeric * 1024);
}

function parseTrafficLimit(value) {
    if (!value || value === '0') return 0;
    const text = String(value).trim().toUpperCase();
    const numeric = parseFloat(text);
    if (!Number.isFinite(numeric)) return 0;
    if (text.endsWith('TB')) return Math.round(numeric * 1024 * 1024 * 1024 * 1024);
    if (text.endsWith('GB')) return Math.round(numeric * 1024 * 1024 * 1024);
    if (text.endsWith('MB')) return Math.round(numeric * 1024 * 1024);
    if (text.endsWith('KB')) return Math.round(numeric * 1024);
    return Math.round(numeric * 1024 * 1024 * 1024);
}

function formatTrafficLimitInput(bytes) {
    if (!bytes || bytes <= 0) return '';
    if (bytes >= 1024 * 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024 * 1024)).toFixed(1)}TB`;
    if (bytes >= 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)}GB`;
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)}MB`;
    return `${(bytes / 1024).toFixed(0)}KB`;
}

function copyToClipboard(text) {
    return navigator.clipboard.writeText(text).then(
        () => Toast.success('Copied to clipboard'),
        () => {
            const textarea = document.createElement('textarea');
            textarea.value = text;
            document.body.appendChild(textarea);
            textarea.select();
            document.execCommand('copy');
            textarea.remove();
            Toast.success('Copied to clipboard');
        },
    );
}
