// Users Component
function renderUsers() {
    const content = document.getElementById('page-content');
    const user = API.getUser();
    const isAdmin = user && user.role === 'admin';

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>用户管理</h2>
                    <p class="subtitle">管理面板账号与个人密码</p>
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

                ${isAdmin ? `
                <div class="card">
                    <h3 style="margin-bottom:16px;">添加用户</h3>
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
                    <button class="btn btn-primary" onclick="createPanelUser()">创建用户</button>
                </div>
                ` : `
                <div class="card" style="display:flex;align-items:center;justify-content:center;">
                    <p style="color:var(--text-muted);">只有管理员可以添加或删除用户</p>
                </div>
                `}
            </div>

            ${isAdmin ? `
            <div class="table-container desktop-only">
                <div class="table-header">
                    <h3>全部用户</h3>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>用户名</th>
                            <th>角色</th>
                            <th>创建时间</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="users-body">
                        <tr><td colspan="5" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">全部用户</h3>
                <div class="m-card-list" id="users-cards">
                    <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                </div>
            </div>
            ` : ''}
        </div>
    `;

    bindPasswordSanitizers(content);

    if (isAdmin) {
        loadUserList();
    }
}

function getUserRoleBadgeClass(role) {
    return role === 'admin' ? 'running' : 'pending';
}

async function loadUserList() {
    try {
        const res = await API.getUsers();
        const users = res.users || [];
        const body = document.getElementById('users-body');
        const currentUser = API.getUser();

        const cards = document.getElementById('users-cards');

        if (!body && !cards) return;

        if (users.length === 0) {
            if (body) body.innerHTML = '<tr><td colspan="5" class="empty-state"><p>暂无用户</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无用户</p>';
            return;
        }

        if (body) body.innerHTML = users.map(user => `
            <tr>
                <td>#${user.id}</td>
                <td>${escHTML(user.username)}</td>
                <td><span class="badge badge-${getUserRoleBadgeClass(user.role)}">${user.role}</span></td>
                <td>${new Date(user.created_at).toLocaleString()}</td>
                <td>
                    ${currentUser && currentUser.id !== user.id ? `
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, '${escHTML(user.username)}')">删除</button>
                    ` : '<span style="color:var(--text-muted);font-size:0.8rem;">当前用户</span>'}
                </td>
            </tr>
        `).join('');

        if (cards) cards.innerHTML = users.map(user => `
            <div class="m-card" style="padding:10px;">
                <div style="display:flex;align-items:center;justify-content:space-between;">
                    <div style="display:flex;align-items:center;gap:8px;">
                        <strong>${escHTML(user.username)}</strong>
                        <span class="badge badge-${getUserRoleBadgeClass(user.role)}" style="font-size:0.68rem;padding:2px 6px;">${user.role}</span>
                    </div>
                    ${currentUser && currentUser.id !== user.id ? `
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${user.id}, '${escHTML(user.username)}')">删除</button>
                    ` : '<span style="color:var(--text-muted);font-size:0.75rem;">当前</span>'}
                </div>
                <div style="font-size:0.75rem;color:var(--text-muted);margin-top:4px;">${new Date(user.created_at).toLocaleString()}</div>
            </div>
        `).join('');
    } catch (err) {
        Toast.error('加载用户列表失败: ' + err.message);
    }
}

async function createPanelUser() {
    const username = document.getElementById('new-user-name').value.trim();
    const password = normalizePasswordValue(document.getElementById('new-user-password').value);
    const confirm = normalizePasswordValue(document.getElementById('new-user-confirm').value);
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

    try {
        const res = await API.createUser(username, password);
        Toast.success(`用户 ${res.user.username} 已创建`);
        document.getElementById('new-user-name').value = '';
        document.getElementById('new-user-password').value = '';
        document.getElementById('new-user-confirm').value = '';
        loadUserList();
    } catch (err) {
        Toast.error('创建用户失败: ' + err.message);
    }
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
