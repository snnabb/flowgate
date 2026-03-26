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
                    <h2>${isAdmin ? 'User Management' : 'My Account'}</h2>
                    <p class="subtitle">${isAdmin ? 'Create accounts and assign node quotas' : 'View your node quotas and rule bandwidths'}</p>
                </div>
            </div>

            <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px;margin-bottom:16px;">
                <div class="card">
                    <h3 style="margin-bottom:16px;">Change Password</h3>
                    <div class="form-group">
                        <label>Current password</label>
                        <input type="password" class="form-input" id="old-pwd" placeholder="Current password">
                    </div>
                    <div class="form-group">
                        <label>New password</label>
                        <input type="password" class="form-input" id="new-pwd" placeholder="New password">
                    </div>
                    <div class="form-group">
                        <label>Confirm new password</label>
                        <input type="password" class="form-input" id="confirm-pwd" placeholder="Repeat new password">
                    </div>
                    <button class="btn btn-primary" onclick="changeMyPassword()">Update password</button>
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
            <h3 style="margin-bottom:16px;">Create User</h3>
            <div class="form-group">
                <label>Username</label>
                <input type="text" class="form-input" id="new-user-name" placeholder="New username">
            </div>
            <div class="form-group">
                <label>Initial password</label>
                <input type="password" class="form-input" id="new-user-password" placeholder="At least 6 characters">
            </div>
            <div class="form-group">
                <label>Confirm password</label>
                <input type="password" class="form-input" id="new-user-confirm" placeholder="Repeat password">
            </div>
            <button class="btn btn-primary" onclick="createPanelUser()">Create user</button>
        </div>
    `;
}

function renderSelfSummaryCard() {
    return `
        <div class="card">
            <h3 style="margin-bottom:16px;">Quota Summary</h3>
            <div class="stats-grid" style="grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;">
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-assigned-nodes">-</div>
                    <div class="stat-label">Assigned Nodes</div>
                </div>
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-remaining-traffic">-</div>
                    <div class="stat-label">Remaining Traffic</div>
                </div>
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-total-rules">-</div>
                    <div class="stat-label">My Rules</div>
                </div>
                <div class="stat-card" style="padding:16px;">
                    <div class="stat-value" id="self-total-traffic">-</div>
                    <div class="stat-label">My Traffic</div>
                </div>
            </div>
        </div>
    `;
}

function renderAdminUserTable() {
    return `
        <div class="table-container desktop-only">
            <div class="table-header">
                <h3>Users</h3>
            </div>
            <table>
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Username</th>
                        <th>Status</th>
                        <th>Assigned Nodes</th>
                        <th>Created</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody id="users-body">
                    <tr><td colspan="6" class="empty-state"><p>Loading...</p></td></tr>
                </tbody>
            </table>
        </div>
        <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
            <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">Users</h3>
            <div class="m-card-list" id="users-cards">
                <p style="color:var(--text-muted);font-size:0.85rem;">Loading...</p>
            </div>
        </div>
    `;
}

function renderSelfQuotaSection() {
    return `
        <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px;">
            <div class="table-container desktop-only">
                <div class="table-header">
                    <h3>Assigned Nodes</h3>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>Node</th>
                            <th>Traffic Used</th>
                            <th>Traffic Remaining</th>
                            <th>Default Bandwidth</th>
                        </tr>
                    </thead>
                    <tbody id="self-access-body">
                        <tr><td colspan="4" class="empty-state"><p>Loading...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">Assigned Nodes</h3>
                <div class="m-card-list" id="self-access-cards">
                    <p style="color:var(--text-muted);font-size:0.85rem;">Loading...</p>
                </div>
            </div>

            <div class="table-container desktop-only">
                <div class="table-header">
                    <h3>My Rules</h3>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Node</th>
                            <th>Bandwidth</th>
                            <th>Traffic</th>
                        </tr>
                    </thead>
                    <tbody id="self-rules-body">
                        <tr><td colspan="4" class="empty-state"><p>Loading...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">My Rules</h3>
                <div class="m-card-list" id="self-rules-cards">
                    <p style="color:var(--text-muted);font-size:0.85rem;">Loading...</p>
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
        Toast.error(`Failed to load users: ${error.message}`);
    }
}

function renderAdminUsersTable() {
    const body = document.getElementById('users-body');
    const cards = document.getElementById('users-cards');
    const currentUser = API.getUser();

    if (!panelUsersCache.length) {
        if (body) body.innerHTML = '<tr><td colspan="6" class="empty-state"><p>No users yet</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">No users yet</p>';
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
                            <button class="btn btn-sm btn-secondary" onclick="showEditUserModal(${user.id})">Edit</button>
                            ${currentUser && currentUser.id !== user.id ? `<button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, ${JSON.stringify(user.username)})">Delete</button>` : ''}
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
                            <span class="m-card-label">Assigned Nodes</span>
                            <span class="m-card-val">${access.length}</span>
                        </div>
                        <div class="m-card-row">
                            <span class="m-card-label">Created</span>
                            <span class="m-card-val">${new Date(user.created_at).toLocaleString()}</span>
                        </div>
                    </div>
                    <div class="m-card-foot">
                        <div class="action-group" style="display:flex;gap:6px;">
                            <button class="btn btn-sm btn-secondary" onclick="showEditUserModal(${user.id})">Edit</button>
                            <button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, ${JSON.stringify(user.username)})">Delete</button>
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
        Toast.error(`Failed to load account details: ${error.message}`);
    }
}

function renderSelfAccess(access) {
    const body = document.getElementById('self-access-body');
    const cards = document.getElementById('self-access-cards');

    if (!access.length) {
        if (body) body.innerHTML = '<tr><td colspan="4" class="empty-state"><p>No assigned nodes</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">No assigned nodes</p>';
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
                        <span class="m-card-label">Traffic Used</span>
                        <span class="m-card-val">${formatNodeTrafficUsage(item)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">Remaining</span>
                        <span class="m-card-val">${formatNodeRemaining(item)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">Default Bandwidth</span>
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
        if (body) body.innerHTML = '<tr><td colspan="4" class="empty-state"><p>No rules yet</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">No rules yet</p>';
        return;
    }

    if (body) {
        body.innerHTML = rules.map((rule) => `
            <tr>
                <td>${escHTML(rule.name || `Rule #${rule.id}`)}</td>
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
                    <strong>${escHTML(rule.name || `Rule #${rule.id}`)}</strong>
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">Node</span>
                        <span class="m-card-val">${escHTML(nodeNames[rule.node_id] || `#${rule.node_id}`)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">Bandwidth</span>
                        <span class="m-card-val">${formatBandwidthLimit(rule.speed_limit)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">Traffic</span>
                        <span class="m-card-val">${formatBytes((rule.traffic_in || 0) + (rule.traffic_out || 0))}</span>
                    </div>
                </div>
            </div>
        `).join('');
    }
}

function renderUserStatusBadge(enabled) {
    return `<span class="badge badge-${enabled ? 'running' : 'error'}">${enabled ? 'Enabled' : 'Disabled'}</span>`;
}

function formatNodeTrafficUsage(item) {
    if (!item.traffic_quota || item.traffic_quota <= 0) {
        return `${formatBytes(item.traffic_used || 0)} / Unlimited`;
    }
    return formatTrafficWithLimit(item.traffic_used || 0, item.traffic_quota);
}

function formatNodeRemaining(item) {
    if (!item.traffic_quota || item.traffic_quota <= 0) {
        return 'Unlimited';
    }
    return formatBytes(Math.max(0, (item.traffic_quota || 0) - (item.traffic_used || 0)));
}

function formatRemainingTraffic(remainingTraffic, access) {
    const hasLimitedAccess = (access || []).some((item) => (item.traffic_quota || 0) > 0);
    if (!hasLimitedAccess) {
        return 'Unlimited';
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
        Toast.error('Username and password are required');
        return;
    }
    if (password.length < 6) {
        Toast.error('Password must be at least 6 characters');
        return;
    }
    if (password !== confirm) {
        Toast.error('Passwords do not match');
        return;
    }

    try {
        await API.createUser({ username, password, role: 'user' });
        Toast.success('User created');
        document.getElementById('new-user-name').value = '';
        document.getElementById('new-user-password').value = '';
        document.getElementById('new-user-confirm').value = '';
        loadAdminUserPanel();
    } catch (error) {
        Toast.error(`Failed to create user: ${error.message}`);
    }
}

async function showEditUserModal(userId) {
    const user = panelUsersCache.find((entry) => entry.id === userId);
    if (!user) {
        Toast.error('User not found');
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
        `Edit ${user.username}`,
        `
            <div class="form-group">
                <label>Account status</label>
                <label style="display:flex;align-items:center;gap:8px;font-weight:500;">
                    <input type="checkbox" id="edit-user-enabled" ${user.enabled ? 'checked' : ''}>
                    Enabled
                </label>
            </div>
            <div class="form-group">
                <label>Node permissions</label>
                <div style="display:flex;flex-direction:column;gap:10px;max-height:360px;overflow:auto;">
                    ${nodes.map((node) => {
                        const row = accessMap[node.id];
                        return `
                            <label style="border:1px solid var(--border-color);border-radius:12px;padding:12px;display:flex;flex-direction:column;gap:10px;">
                                <div style="display:flex;align-items:center;justify-content:space-between;gap:12px;">
                                    <span style="font-weight:600;">${escHTML(node.name)}</span>
                                    <span style="display:flex;align-items:center;gap:8px;">
                                        <input type="checkbox" data-access-enabled="${node.id}" ${row ? 'checked' : ''}>
                                        Assign
                                    </span>
                                </div>
                                <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;">
                                    <div>
                                        <div style="font-size:0.78rem;color:var(--text-muted);margin-bottom:4px;">Total traffic quota</div>
                                        <input type="text" class="form-input" data-access-quota="${node.id}" value="${formatTrafficLimitInput(row?.traffic_quota || 0)}" placeholder="Unlimited">
                                    </div>
                                    <div>
                                        <div style="font-size:0.78rem;color:var(--text-muted);margin-bottom:4px;">Default bandwidth (M)</div>
                                        <input type="number" class="form-input" data-access-bandwidth="${node.id}" value="${row ? bandwidthKBToM(row.bandwidth_limit) : ''}" min="0" step="0.1" placeholder="Unlimited">
                                    </div>
                                </div>
                                ${row ? `<div class="mini-meta">Used ${formatBytes(row.traffic_used || 0)} / ${row.traffic_quota > 0 ? formatBytes(row.traffic_quota) : 'Unlimited'}</div>` : ''}
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
                }];
            });

            try {
                await API.updateUser(userId, { enabled });
                await API.replaceUserAccess(userId, { access: nextAccess });
                closeModal();
                Toast.success('User updated');
                loadAdminUserPanel();
            } catch (error) {
                Toast.error(`Failed to update user: ${error.message}`);
            }
        },
        'Cancel',
        'Save',
    );
}

async function confirmDeleteUser(id, username) {
    showModal(
        'Delete User',
        `<p style="color:var(--color-danger);">Delete <strong>${escHTML(username)}</strong>?</p>`,
        async () => {
            try {
                await API.deleteUser(id);
                closeModal();
                Toast.success('User deleted');
                loadAdminUserPanel();
            } catch (error) {
                Toast.error(`Failed to delete user: ${error.message}`);
            }
        },
        'Cancel',
        'Delete',
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
        Toast.error('Password fields are required');
        return;
    }
    if (newPwd.length < 6) {
        Toast.error('Password must be at least 6 characters');
        return;
    }
    if (newPwd !== confirmPwd) {
        Toast.error('Passwords do not match');
        return;
    }

    try {
        await API.changePassword(oldPwd, newPwd);
        Toast.success('Password updated');
        document.getElementById('old-pwd').value = '';
        document.getElementById('new-pwd').value = '';
        document.getElementById('confirm-pwd').value = '';
    } catch (error) {
        Toast.error(`Failed to update password: ${error.message}`);
    }
}
