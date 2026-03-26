// Users Component
let _visibleUsersCache = [];
let _visibleUsersMap = {};

function renderUsers() {
    const content = document.getElementById('page-content');
    const currentUser = API.getUser();
    const isManager = currentUser && isManagerRole(currentUser.role);

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>用户管理</h2>
                    <p class="subtitle">管理账户、归属关系和配额限制</p>
                </div>
                <button class="btn btn-secondary mobile-only" onclick="handleLogout()">退出登录</button>
            </div>

            <div style="display:grid; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); gap:16px; margin-bottom:16px;">
                <div class="card">
                    <h3 style="margin-bottom:16px;">修改我的密码</h3>
                    <div class="form-group">
                        <label>旧密码</label>
                        <input type="password" class="form-input" id="old-pwd" placeholder="输入旧密码">
                    </div>
                    <div class="form-group">
                        <label>新密码</label>
                        <input type="password" class="form-input" id="new-pwd" placeholder="输入新密码">
                    </div>
                    <div class="form-group">
                        <label>确认新密码</label>
                        <input type="password" class="form-input" id="confirm-pwd" placeholder="再次输入新密码">
                    </div>
                    <button class="btn btn-primary" onclick="changeMyPassword()">修改密码</button>
                </div>

                ${isManager ? renderUserCreateCard(currentUser) : `
                <div class="card" style="display:flex;align-items:center;justify-content:center;">
                    <p style="color:var(--text-muted);">当前账号只有自助密码修改权限，用户创建与配额管理仅开放给 admin / reseller。</p>
                </div>
                `}
            </div>

            ${isManager ? `
            <div class="table-container desktop-only">
                <div class="table-header">
                    <h3>可见用户</h3>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>用户名</th>
                            <th>角色</th>
                            <th>上级</th>
                            <th>流量配额</th>
                            <th>倍率</th>
                            <th>到期时间</th>
                            <th>限制</th>
                            <th>创建时间</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="users-body">
                        <tr><td colspan="10" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">可见用户</h3>
                <div class="m-card-list" id="users-cards">
                    <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                </div>
            </div>
            ` : ''}
        </div>
    `;

    bindPasswordSanitizers(content);

    if (isManager) {
        loadUserList();
    }
}

function renderUserCreateCard(currentUser) {
    const isAdmin = currentUser && isAdminRole(currentUser.role);
    const fixedRole = isAdmin ? '' : 'user';
    const fixedParent = currentUser ? currentUser.id : '';

    return `
        <div class="card">
            <h3 style="margin-bottom:16px;">创建用户</h3>
            <div class="form-group">
                <label>用户名</label>
                <input type="text" class="form-input" id="new-user-name" placeholder="输入新用户名">
            </div>
            <div class="form-group">
                <label>初始密码</label>
                <input type="password" class="form-input" id="new-user-password" placeholder="至少 6 位">
            </div>
            <div class="form-group">
                <label>确认密码</label>
                <input type="password" class="form-input" id="new-user-confirm" placeholder="再次输入初始密码">
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>角色</label>
                    <select class="form-select" id="new-user-role" ${isAdmin ? '' : 'disabled'} onchange="syncUserCreateFormRole()">
                        <option value="admin" ${fixedRole === 'admin' ? 'selected' : ''}>admin</option>
                        <option value="reseller" ${fixedRole === 'reseller' ? 'selected' : ''}>reseller</option>
                        <option value="user" ${fixedRole === 'user' ? 'selected' : ''}>user</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>上级账号</label>
                    <select class="form-select" id="new-user-parent" ${isAdmin ? '' : 'disabled'}>
                        ${isAdmin ? '<option value="">不指定</option>' : ''}
                        ${fixedParent ? `<option value="${fixedParent}" selected>${escHTML(currentUser.username)}</option>` : ''}
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>流量配额</label>
                    <input type="text" class="form-input" id="new-user-traffic-quota" placeholder="例如 500GB，0 表示不限">
                </div>
                <div class="form-group">
                    <label>流量倍率</label>
                    <input type="number" class="form-input" id="new-user-ratio" value="1" min="0.1" step="0.1">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>到期时间</label>
                    <input type="datetime-local" class="form-input" id="new-user-expires-at">
                </div>
                <div class="form-group">
                    <label>规则上限</label>
                    <input type="number" class="form-input" id="new-user-max-rules" value="0" min="0">
                </div>
            </div>
            <div class="form-group">
                <label>带宽上限</label>
                <input type="number" class="form-input" id="new-user-bandwidth-limit" value="0" min="0" placeholder="KB/s，0 表示不限">
            </div>
            <div class="mini-meta" id="new-user-scope-note">
                创建后资源范围将按当前账号角色自动收敛。
            </div>
            <button class="btn btn-primary" onclick="createPanelUser()">创建用户</button>
        </div>
    `;
}

function getUserRoleBadgeClass(role) {
    switch (role) {
        case 'admin':
            return 'admin';
        case 'reseller':
            return 'reseller';
        default:
            return 'user';
    }
}

function syncUserCreateFormRole() {
    const currentUser = API.getUser();
    const roleSelect = document.getElementById('new-user-role');
    const parentSelect = document.getElementById('new-user-parent');
    const note = document.getElementById('new-user-scope-note');
    if (!currentUser || !roleSelect || !parentSelect) return;

    if (!isAdminRole(currentUser.role)) {
        roleSelect.value = 'user';
        parentSelect.value = String(currentUser.id);
        if (note) {
            note.textContent = 'reseller 只能创建直属 user 账号，parent 会自动绑定到当前 reseller。';
        }
        return;
    }

    const role = roleSelect.value;
    if (role === 'admin') {
        parentSelect.value = '';
    }
    if (note) {
        note.textContent = role === 'reseller'
            ? 'reseller 可继续管理直属 user；如不指定上级，则该 reseller 为顶级账号。'
            : role === 'user'
                ? '普通 user 只能看到自身资源；建议为 user 选择上级 reseller 以便授权管理。'
                : 'admin 拥有全局管理权限，parent 可留空。';
    }
}

async function loadUserList() {
    try {
        const res = await API.getUsers();
        _visibleUsersCache = res.users || [];
        _visibleUsersMap = buildUserMap(_visibleUsersCache);
        hydrateUserParentOptions();

        const body = document.getElementById('users-body');
        const cards = document.getElementById('users-cards');
        const currentUser = API.getUser();

        if (!body && !cards) return;

        if (_visibleUsersCache.length === 0) {
            if (body) body.innerHTML = '<tr><td colspan="10" class="empty-state"><p>暂无用户</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无用户</p>';
            return;
        }

        if (body) body.innerHTML = _visibleUsersCache.map(user => `
            <tr>
                <td>#${user.id}</td>
                <td><strong>${escHTML(user.username)}</strong></td>
                <td><span class="badge badge-${getUserRoleBadgeClass(user.role)}">${user.role}</span></td>
                <td>${escHTML(resolveUserLabel(user.parent_id, _visibleUsersMap, currentUser))}</td>
                <td>${formatAccountTrafficQuota(user.traffic_quota, user.traffic_used)}</td>
                <td>${Number(user.ratio || 1).toFixed(1)}x</td>
                <td>${formatNullableDateTime(user.expires_at)}</td>
                <td>
                    <div>Rules: ${user.max_rules > 0 ? user.max_rules : 'Unlimited'}</div>
                    <div style="color:var(--text-muted);font-size:0.78rem;">Bandwidth: ${formatBandwidthLimit(user.bandwidth_limit)}</div>
                </td>
                <td>${new Date(user.created_at).toLocaleString()}</td>
                <td>
                    ${currentUser && currentUser.id !== user.id ? `
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, '${escHTML(user.username)}')">删除</button>
                    ` : '<span style="color:var(--text-muted);font-size:0.8rem;">当前用户</span>'}
                </td>
            </tr>
        `).join('');

        if (cards) cards.innerHTML = _visibleUsersCache.map(user => `
            <div class="m-card" style="padding:10px;">
                <div class="m-card-head">
                    <div style="display:flex;align-items:center;gap:8px;">
                        <strong>${escHTML(user.username)}</strong>
                        <span class="badge badge-${getUserRoleBadgeClass(user.role)}" style="font-size:0.68rem;padding:2px 6px;">${user.role}</span>
                    </div>
                    ${currentUser && currentUser.id !== user.id ? `
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, '${escHTML(user.username)}')">删除</button>
                    ` : '<span style="color:var(--text-muted);font-size:0.75rem;">当前</span>'}
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">上级</span>
                        <span class="m-card-val">${escHTML(resolveUserLabel(user.parent_id, _visibleUsersMap, currentUser))}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">流量配额</span>
                        <span class="m-card-val">${formatAccountTrafficQuota(user.traffic_quota, user.traffic_used)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">倍率</span>
                        <span class="m-card-val">${Number(user.ratio || 1).toFixed(1)}x</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">到期</span>
                        <span class="m-card-val">${formatNullableDateTime(user.expires_at)}</span>
                    </div>
                    <div class="m-card-row full">
                        <span class="m-card-label">限制</span>
                        <span class="m-card-val">Rules: ${user.max_rules > 0 ? user.max_rules : 'Unlimited'} / Bandwidth: ${formatBandwidthLimit(user.bandwidth_limit)}</span>
                    </div>
                </div>
            </div>
        `).join('');
    } catch (err) {
        Toast.error('加载用户列表失败: ' + err.message);
    }
}

function hydrateUserParentOptions() {
    const currentUser = API.getUser();
    const parentSelect = document.getElementById('new-user-parent');
    if (!parentSelect || !currentUser) return;

    if (!isAdminRole(currentUser.role)) {
        parentSelect.innerHTML = `<option value="${currentUser.id}" selected>${escHTML(currentUser.username)}</option>`;
        return;
    }

    const currentValue = parentSelect.value;
    const options = ['<option value="">不指定</option>'].concat(
        _visibleUsersCache.map(user => `<option value="${user.id}">${escHTML(user.username)} (${user.role})</option>`)
    );
    parentSelect.innerHTML = options.join('');
    parentSelect.value = currentValue || '';
    syncUserCreateFormRole();
}

async function createPanelUser() {
    const currentUser = API.getUser();
    const username = document.getElementById('new-user-name').value.trim();
    const password = normalizePasswordValue(document.getElementById('new-user-password').value);
    const confirm = normalizePasswordValue(document.getElementById('new-user-confirm').value);
    const roleInput = document.getElementById('new-user-role');
    const parentInput = document.getElementById('new-user-parent');
    const quotaInput = document.getElementById('new-user-traffic-quota');
    const ratioInput = document.getElementById('new-user-ratio');
    const expiresInput = document.getElementById('new-user-expires-at');
    const maxRulesInput = document.getElementById('new-user-max-rules');
    const bandwidthInput = document.getElementById('new-user-bandwidth-limit');

    document.getElementById('new-user-password').value = password;
    document.getElementById('new-user-confirm').value = confirm;

    if (!username || !password) {
        Toast.error('请填写完整的用户名和密码');
        return;
    }

    if (password.length < 6) {
        Toast.error('密码至少 6 位');
        return;
    }

    if (password !== confirm) {
        Toast.error('两次密码不一致');
        return;
    }

    const payload = {
        username,
        password,
        role: roleInput ? roleInput.value : 'user',
        traffic_quota: parseTrafficLimit(quotaInput?.value),
        ratio: parseFloat(ratioInput?.value || '1') || 1,
        max_rules: parseInt(maxRulesInput?.value || '0', 10) || 0,
        bandwidth_limit: parseInt(bandwidthInput?.value || '0', 10) || 0,
    };

    if (!isAdminRole(currentUser.role)) {
        payload.role = 'user';
        payload.parent_id = currentUser.id;
    } else if (parentInput && parentInput.value) {
        payload.parent_id = parseInt(parentInput.value, 10);
    }

    if (expiresInput && expiresInput.value) {
        payload.expires_at = new Date(expiresInput.value).toISOString();
    }

    try {
        const res = await API.createUser(payload);
        Toast.success(`用户 ${res.user.username} 已创建`);
        resetCreateUserForm(currentUser);
        loadUserList();
    } catch (err) {
        Toast.error('创建用户失败: ' + err.message);
    }
}

function resetCreateUserForm(currentUser) {
    document.getElementById('new-user-name').value = '';
    document.getElementById('new-user-password').value = '';
    document.getElementById('new-user-confirm').value = '';
    document.getElementById('new-user-traffic-quota').value = '';
    document.getElementById('new-user-ratio').value = '1';
    document.getElementById('new-user-expires-at').value = '';
    document.getElementById('new-user-max-rules').value = '0';
    document.getElementById('new-user-bandwidth-limit').value = '0';

    const roleSelect = document.getElementById('new-user-role');
    const parentSelect = document.getElementById('new-user-parent');

    if (roleSelect) {
        roleSelect.value = isAdminRole(currentUser.role) ? 'user' : 'user';
    }
    if (parentSelect) {
        parentSelect.value = isAdminRole(currentUser.role) ? '' : String(currentUser.id);
    }
    syncUserCreateFormRole();
}

async function changeMyPassword() {
    const oldPwd = normalizePasswordValue(document.getElementById('old-pwd').value);
    const newPwd = normalizePasswordValue(document.getElementById('new-pwd').value);
    const confirmPwd = normalizePasswordValue(document.getElementById('confirm-pwd').value);
    document.getElementById('old-pwd').value = oldPwd;
    document.getElementById('new-pwd').value = newPwd;
    document.getElementById('confirm-pwd').value = confirmPwd;

    if (!oldPwd || !newPwd) {
        Toast.error('请填写完整的密码信息');
        return;
    }
    if (newPwd !== confirmPwd) {
        Toast.error('两次密码不一致');
        return;
    }
    if (newPwd.length < 6) {
        Toast.error('密码至少 6 位');
        return;
    }

    try {
        await API.changePassword(oldPwd, newPwd);
        Toast.success('密码修改成功');
        document.getElementById('old-pwd').value = '';
        document.getElementById('new-pwd').value = '';
        document.getElementById('confirm-pwd').value = '';
    } catch (err) {
        Toast.error('修改失败: ' + err.message);
    }
}

async function confirmDeleteUser(id, name) {
    showModal('删除用户', `
        <p style="color:var(--color-danger);">确定要删除用户 <strong>${name}</strong> 吗？</p>
    `, async () => {
        try {
            await API.deleteUser(id);
            closeModal();
            Toast.success('用户已删除');
            loadUserList();
        } catch (err) {
            Toast.error('删除失败: ' + err.message);
        }
    }, '取消', '确认删除');
}
