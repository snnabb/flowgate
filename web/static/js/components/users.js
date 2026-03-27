let panelUsersCache = [];
let panelNodesCache = [];
let panelUserAccessCache = {};

function renderUsers() {
    const content = document.getElementById('page-content');
    const currentUser = API.getUser();
    const isAdmin = currentUser && isAdminRole(currentUser.role);

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>${isAdmin ? '用户管理' : '我的账号'}</h2>
                    <p class="subtitle">${isAdmin ? '创建用户并配置节点权限、流量和带宽' : '查看你的节点配额、剩余流量和规则带宽'}</p>
                </div>
            </div>

            <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px;margin-bottom:16px;">
                <div class="card">
                    <h3 style="margin-bottom:16px;">修改密码</h3>
                    <div class="form-group">
                        <label>当前密码</label>
                        <input type="password" class="form-input" id="old-pwd" placeholder="当前密码">
                    </div>
                    <div class="form-group">
                        <label>新密码</label>
                        <input type="password" class="form-input" id="new-pwd" placeholder="新密码">
                    </div>
                    <div class="form-group">
                        <label>确认新密码</label>
                        <input type="password" class="form-input" id="confirm-pwd" placeholder="再次输入新密码">
                    </div>
                    <button class="btn btn-primary" onclick="changeMyPassword()">更新密码</button>
                </div>
                ${isAdmin ? renderCreateUserCard() : renderSelfSummaryCard()}
            </div>

            ${isAdmin ? renderAdminUserTable() : renderSelfQuotaSection()}
        </div>
    `;

    bindPasswordSanitizers(content);
    if (isAdmin) {
        loadAdminUserPanel();
    } else {
        loadSelfUserPanel();
    }
}

function renderCreateUserCard() {
    return `
        <div class="card">
            <h3 style="margin-bottom:16px;">创建用户</h3>
            <div class="form-group">
                <label>用户名</label>
                <input type="text" class="form-input" id="new-user-name" placeholder="输入用户名">
            </div>
            <div class="form-group">
                <label>初始密码</label>
                <input type="password" class="form-input" id="new-user-password" placeholder="至少 6 位">
            </div>
            <div class="form-group">
                <label>确认密码</label>
                <input type="password" class="form-input" id="new-user-confirm" placeholder="再次输入密码">
            </div>
            <button class="btn btn-primary" onclick="createPanelUser()">创建用户</button>
        </div>
    `;
}

function renderSelfSummaryCard() {
    return `
        <div class="card">
            <h3 style="margin-bottom:16px;">额度概览</h3>
            <div class="stats-grid" style="grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;">
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-assigned-nodes">-</div>
                    <div class="stat-label">已授权节点</div>
                </div>
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-remaining-traffic">-</div>
                    <div class="stat-label">剩余流量</div>
                </div>
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-total-rules">-</div>
                    <div class="stat-label">我的规则</div>
                </div>
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-total-traffic">-</div>
                    <div class="stat-label">我的流量</div>
                </div>
            </div>
        </div>
    `;
}

function renderAdminUserTable() {
    return `
        <div class="table-container desktop-only">
            <div class="table-header">
                <h3>用户列表</h3>
            </div>
            <table>
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>用户名</th>
                        <th>状态</th>
                        <th>已授权节点</th>
                        <th>创建时间</th>
                        <th>操作</th>
                    </tr>
                </thead>
                <tbody id="users-body">
                    <tr><td colspan="6" class="empty-state"><p>加载中...</p></td></tr>
                </tbody>
            </table>
        </div>
        <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
            <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">用户列表</h3>
            <div class="m-card-list" id="users-cards">
                <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
            </div>
        </div>
    `;
}

function renderSelfQuotaSection() {
    return `
        <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px;">
            <div class="table-container desktop-only">
                <div class="table-header">
                    <h3>已授权节点</h3>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>节点</th>
                            <th>已用流量</th>
                            <th>剩余流量</th>
                            <th>默认带宽</th>
                        </tr>
                    </thead>
                    <tbody id="self-access-body">
                        <tr><td colspan="4" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">已授权节点</h3>
                <div class="m-card-list" id="self-access-cards">
                    <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                </div>
            </div>

            <div class="table-container desktop-only">
                <div class="table-header">
                    <h3>我的规则</h3>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>名称</th>
                            <th>节点</th>
                            <th>带宽</th>
                            <th>流量</th>
                        </tr>
                    </thead>
                    <tbody id="self-rules-body">
                        <tr><td colspan="4" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">我的规则</h3>
                <div class="m-card-list" id="self-rules-cards">
                    <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                </div>
            </div>
        </div>
    `;
}

async function loadAdminUserPanel() {
    try {
        const [usersRes, nodesRes] = await Promise.all([
            API.getUsers(),
            API.getNodes(),
        ]);
        panelUsersCache = (usersRes.users || []).filter((user) => user.role !== 'admin');
        panelNodesCache = nodesRes.nodes || [];

        const accessEntries = await Promise.all(
            panelUsersCache.map(async (user) => {
                try {
                    const res = await API.getUserAccess(user.id);
                    return [user.id, res.access || []];
                } catch {
                    return [user.id, []];
                }
            }),
        );
        panelUserAccessCache = Object.fromEntries(accessEntries);
        renderAdminUsersTable();
    } catch (error) {
        Toast.error(`加载用户失败：${error.message}`);
    }
}

function renderAdminUsersTable() {
    const body = document.getElementById('users-body');
    const cards = document.getElementById('users-cards');
    const currentUser = API.getUser();

    if (!panelUsersCache.length) {
        if (body) body.innerHTML = '<tr><td colspan="6" class="empty-state"><p>暂无用户</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无用户</p>';
        return;
    }

    if (body) {
        body.innerHTML = panelUsersCache.map((user) => {
            const access = panelUserAccessCache[user.id] || [];
            return `
                <tr>
                    <td>#${user.id}</td>
                    <td><strong>${escHTML(user.username)}</strong></td>
                    <td>${renderUserStatusBadge(user.enabled)}</td>
                    <td>${access.length}</td>
                    <td>${new Date(user.created_at).toLocaleString()}</td>
                    <td>
                        <div class="action-group">
                            <button class="btn btn-sm btn-secondary" onclick="showEditUserModal(${user.id})">编辑</button>
                            ${currentUser && currentUser.id !== user.id ? `<button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, ${JSON.stringify(user.username)})">删除</button>` : ''}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }

    if (cards) {
        cards.innerHTML = panelUsersCache.map((user) => {
            const access = panelUserAccessCache[user.id] || [];
            return `
                <div class="m-card" style="padding:10px;">
                    <div class="m-card-head">
                        <div style="display:flex;align-items:center;gap:8px;">
                            <strong>${escHTML(user.username)}</strong>
                            ${renderUserStatusBadge(user.enabled)}
                        </div>
                    </div>
                    <div class="m-card-body">
                        <div class="m-card-row">
                            <span class="m-card-label">已授权节点</span>
                            <span class="m-card-val">${access.length}</span>
                        </div>
                        <div class="m-card-row">
                            <span class="m-card-label">创建时间</span>
                            <span class="m-card-val">${new Date(user.created_at).toLocaleString()}</span>
                        </div>
                    </div>
                    <div class="m-card-foot">
                        <div class="action-group" style="display:flex;gap:6px;">
                            <button class="btn btn-sm btn-secondary" onclick="showEditUserModal(${user.id})">编辑</button>
                            <button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, ${JSON.stringify(user.username)})">删除</button>
                        </div>
                    </div>
                </div>
            `;
        }).join('');
    }
}

async function loadSelfUserPanel() {
    try {
        const [dashboardRes, accessRes, rulesRes] = await Promise.all([
            API.getDashboard(),
            API.getSelfAccess(),
            API.getRules(),
        ]);
        const stats = dashboardRes.stats || {};
        const access = accessRes.access || [];
        const rules = rulesRes.rules || [];

        document.getElementById('self-assigned-nodes').textContent = stats.assigned_nodes ?? access.length;
        document.getElementById('self-total-rules').textContent = stats.total_rules ?? rules.length;
        document.getElementById('self-total-traffic').textContent = formatBytes((stats.total_traffic_in || 0) + (stats.total_traffic_out || 0));
        document.getElementById('self-remaining-traffic').textContent = formatRemainingTraffic(stats.remaining_traffic, access);

        renderSelfAccess(access);
        renderSelfRules(rules, access);
    } catch (error) {
        Toast.error(`加载账号详情失败：${error.message}`);
    }
}

function renderSelfAccess(access) {
    const body = document.getElementById('self-access-body');
    const cards = document.getElementById('self-access-cards');

    if (!access.length) {
        if (body) body.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无已授权节点</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无已授权节点</p>';
        return;
    }

    if (body) {
        body.innerHTML = access.map((item) => `
            <tr>
                <td>${escHTML(item.node_name || `#${item.node_id}`)}</td>
                <td>${formatNodeTrafficUsage(item)}</td>
                <td>${formatNodeRemaining(item)}</td>
                <td>${formatBandwidthLimit(item.bandwidth_limit)}</td>
            </tr>
        `).join('');
    }

    if (cards) {
        cards.innerHTML = access.map((item) => `
            <div class="m-card" style="padding:10px;">
                <div class="m-card-head">
                    <strong>${escHTML(item.node_name || `#${item.node_id}`)}</strong>
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">已用流量</span>
                        <span class="m-card-val">${formatNodeTrafficUsage(item)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">剩余流量</span>
                        <span class="m-card-val">${formatNodeRemaining(item)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">默认带宽</span>
                        <span class="m-card-val">${formatBandwidthLimit(item.bandwidth_limit)}</span>
                    </div>
                </div>
            </div>
        `).join('');
    }
}

function renderSelfRules(rules, access) {
    const body = document.getElementById('self-rules-body');
    const cards = document.getElementById('self-rules-cards');
    const nodeNames = Object.fromEntries(access.map((item) => [item.node_id, item.node_name || `#${item.node_id}`]));

    if (!rules.length) {
        if (body) body.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无规则</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无规则</p>';
        return;
    }

    if (body) {
        body.innerHTML = rules.map((rule) => `
            <tr>
                <td>${escHTML(rule.name || `规则 #${rule.id}`)}</td>
                <td>${escHTML(nodeNames[rule.node_id] || `#${rule.node_id}`)}</td>
                <td>${formatBandwidthLimit(rule.speed_limit)}</td>
                <td>${formatBytes((rule.traffic_in || 0) + (rule.traffic_out || 0))}</td>
            </tr>
        `).join('');
    }

    if (cards) {
        cards.innerHTML = rules.map((rule) => `
            <div class="m-card" style="padding:10px;">
                <div class="m-card-head">
                    <strong>${escHTML(rule.name || `规则 #${rule.id}`)}</strong>
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">节点</span>
                        <span class="m-card-val">${escHTML(nodeNames[rule.node_id] || `#${rule.node_id}`)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">带宽</span>
                        <span class="m-card-val">${formatBandwidthLimit(rule.speed_limit)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">流量</span>
                        <span class="m-card-val">${formatBytes((rule.traffic_in || 0) + (rule.traffic_out || 0))}</span>
                    </div>
                </div>
            </div>
        `).join('');
    }
}

function renderUserStatusBadge(enabled) {
    return `<span class="badge badge-${enabled ? 'running' : 'error'}">${enabled ? '启用' : '停用'}</span>`;
}

function formatNodeTrafficUsage(item) {
    if (!item.traffic_quota || item.traffic_quota <= 0) {
        return `${formatBytes(item.traffic_used || 0)} / 不限`;
    }
    return formatTrafficWithLimit(item.traffic_used || 0, item.traffic_quota);
}

function formatNodeRemaining(item) {
    if (!item.traffic_quota || item.traffic_quota <= 0) {
        return '不限';
    }
    return formatBytes(Math.max(0, (item.traffic_quota || 0) - (item.traffic_used || 0)));
}

function formatRemainingTraffic(remainingTraffic, access) {
    const hasLimitedAccess = (access || []).some((item) => (item.traffic_quota || 0) > 0);
    if (!hasLimitedAccess) {
        return '不限';
    }
    return formatBytes(remainingTraffic || 0);
}

async function createPanelUser() {
    const username = document.getElementById('new-user-name').value.trim();
    const password = normalizePasswordValue(document.getElementById('new-user-password').value);
    const confirm = normalizePasswordValue(document.getElementById('new-user-confirm').value);

    document.getElementById('new-user-password').value = password;
    document.getElementById('new-user-confirm').value = confirm;

    if (!username || !password) {
        Toast.error('请输入用户名和密码');
        return;
    }
    if (password.length < 6) {
        Toast.error('密码至少需要 6 位');
        return;
    }
    if (password !== confirm) {
        Toast.error('两次输入的密码不一致');
        return;
    }

    try {
        await API.createUser({ username, password, role: 'user' });
        Toast.success('用户已创建');
        document.getElementById('new-user-name').value = '';
        document.getElementById('new-user-password').value = '';
        document.getElementById('new-user-confirm').value = '';
        loadAdminUserPanel();
    } catch (error) {
        Toast.error(`创建用户失败：${error.message}`);
    }
}

async function showEditUserModal(userId) {
    const user = panelUsersCache.find((entry) => entry.id === userId);
    if (!user) {
        Toast.error('用户不存在');
        return;
    }

    const [nodesRes, accessRes] = await Promise.all([
        API.getNodes(),
        API.getUserAccess(userId),
    ]);
    const nodes = nodesRes.nodes || [];
    const access = accessRes.access || [];
    const accessMap = Object.fromEntries(access.map((item) => [item.node_id, item]));

    showModal(
        `编辑用户：${user.username}`,
        `
            <div class="form-group">
                <label>账号状态</label>
                <label style="display:flex;align-items:center;gap:8px;font-weight:500;">
                    <input type="checkbox" id="edit-user-enabled" ${user.enabled ? 'checked' : ''}>
                    启用
                </label>
            </div>
            <div class="form-group">
                <label>节点权限</label>
                <div style="display:flex;flex-direction:column;gap:10px;max-height:360px;overflow:auto;">
                    ${nodes.map((node) => {
                        const row = accessMap[node.id];
                        return `
                            <label style="border:1px solid var(--border-color);border-radius:12px;padding:12px;display:flex;flex-direction:column;gap:10px;">
                                <div style="display:flex;align-items:center;justify-content:space-between;gap:12px;">
                                    <span style="font-weight:600;">${escHTML(node.name)}</span>
                                    <span style="display:flex;align-items:center;gap:8px;">
                                        <input type="checkbox" data-access-enabled="${node.id}" ${row ? 'checked' : ''}>
                                        授权
                                    </span>
                                </div>
                                <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;">
                                    <div>
                                        <div style="font-size:0.78rem;color:var(--text-muted);margin-bottom:4px;">总流量额度</div>
                                        <input type="text" class="form-input" data-access-quota="${node.id}" value="${formatTrafficLimitInput(row?.traffic_quota || 0)}" placeholder="不限">
                                    </div>
                                    <div>
                                        <div style="font-size:0.78rem;color:var(--text-muted);margin-bottom:4px;">默认带宽 (M)</div>
                                        <input type="number" class="form-input" data-access-bandwidth="${node.id}" value="${row ? bandwidthKBToM(row.bandwidth_limit) : ''}" min="0" step="0.1" placeholder="不限">
                                    </div>
                                    <div>
                                        <div style="font-size:0.78rem;color:var(--text-muted);margin-bottom:4px;">每节点规则数</div>
                                        <input type="number" class="form-input" data-access-max-rules="${node.id}" value="${row?.max_rules || ''}" min="0" step="1" placeholder="不限">
                                    </div>
                                </div>
                                ${row ? `<div class="mini-meta">已用 ${formatBytes(row.traffic_used || 0)} / ${row.traffic_quota > 0 ? formatBytes(row.traffic_quota) : '不限'}</div>` : ''}
                            </label>
                        `;
                    }).join('')}
                </div>
            </div>
        `,
        async () => {
            const enabled = !!document.getElementById('edit-user-enabled').checked;
            const nextAccess = nodes.flatMap((node) => {
                const assigned = document.querySelector(`[data-access-enabled="${node.id}"]`)?.checked;
                if (!assigned) return [];
                const quotaText = document.querySelector(`[data-access-quota="${node.id}"]`)?.value || '';
                const bandwidthText = document.querySelector(`[data-access-bandwidth="${node.id}"]`)?.value || '';
                return [{
                    node_id: node.id,
                    traffic_quota: parseTrafficLimit(quotaText),
                    bandwidth_limit: parseBandwidthM(bandwidthText),
                    max_rules: Math.max(0, parseInt(document.querySelector(`[data-access-max-rules="${node.id}"]`)?.value || '0', 10) || 0),
                }];
            });

            try {
                await API.updateUser(userId, { enabled });
                await API.replaceUserAccess(userId, { access: nextAccess });
                closeModal();
                Toast.success('用户已更新');
                loadAdminUserPanel();
            } catch (error) {
                Toast.error(`更新用户失败：${error.message}`);
            }
        },
        '取消',
        '保存',
    );
}

async function confirmDeleteUser(id, username) {
    showModal(
        '删除用户',
        `<p style="color:var(--color-danger);">确认删除 <strong>${escHTML(username)}</strong>？</p>`,
        async () => {
            try {
                await API.deleteUser(id);
                closeModal();
                Toast.success('用户已删除');
                loadAdminUserPanel();
            } catch (error) {
                Toast.error(`删除用户失败：${error.message}`);
            }
        },
        '取消',
        '删除',
    );
}

async function changeMyPassword() {
    const oldPwd = normalizePasswordValue(document.getElementById('old-pwd').value);
    const newPwd = normalizePasswordValue(document.getElementById('new-pwd').value);
    const confirmPwd = normalizePasswordValue(document.getElementById('confirm-pwd').value);

    document.getElementById('old-pwd').value = oldPwd;
    document.getElementById('new-pwd').value = newPwd;
    document.getElementById('confirm-pwd').value = confirmPwd;

    if (!oldPwd || !newPwd) {
        Toast.error('请填写完整密码信息');
        return;
    }
    if (newPwd.length < 6) {
        Toast.error('密码至少需要 6 位');
        return;
    }
    if (newPwd !== confirmPwd) {
        Toast.error('两次输入的密码不一致');
        return;
    }

    try {
        await API.changePassword(oldPwd, newPwd);
        Toast.success('密码已更新');
        document.getElementById('old-pwd').value = '';
        document.getElementById('new-pwd').value = '';
        document.getElementById('confirm-pwd').value = '';
    } catch (error) {
        Toast.error(`更新密码失败：${error.message}`);
    }
}
