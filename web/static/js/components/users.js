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
                    <p class="subtitle">管理面板用户</p>
                </div>
            </div>

            <div style="display:grid; grid-template-columns: 1fr 1fr; gap:16px;">
                <!-- Change Password Card -->
                <div class="card">
                    <h3 style="margin-bottom:16px;">修改密码</h3>
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

                <!-- User List (admin only) -->
                ${isAdmin ? `
                <div class="table-container">
                    <div class="table-header">
                        <h3>所有用户</h3>
                    </div>
                    <table>
                        <thead>
                            <tr>
                                <th>ID</th>
                                <th>用户名</th>
                                <th>角色</th>
                                <th>操作</th>
                            </tr>
                        </thead>
                        <tbody id="users-body">
                            <tr><td colspan="4" class="empty-state"><p>加载中...</p></td></tr>
                        </tbody>
                    </table>
                </div>
                ` : `
                <div class="card" style="display:flex;align-items:center;justify-content:center;">
                    <p style="color:var(--text-muted);">用户列表仅管理员可见</p>
                </div>
                `}
            </div>
        </div>
    `;

    if (isAdmin) loadUserList();
}

async function loadUserList() {
    try {
        const res = await API.getUsers();
        const users = res.users || [];
        const body = document.getElementById('users-body');
        const currentUser = API.getUser();

        body.innerHTML = users.map(u => `
            <tr>
                <td>#${u.id}</td>
                <td>${escHTML(u.username)}</td>
                <td><span class="badge ${u.role === 'admin' ? 'badge-online' : 'badge-offline'}">${u.role}</span></td>
                <td>
                    ${currentUser && currentUser.id !== u.id ? `
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteUser(${u.id}, '${escHTML(u.username)}')">删除</button>
                    ` : '<span style="color:var(--text-muted);font-size:0.8rem;">当前用户</span>'}
                </td>
            </tr>
        `).join('');
    } catch (err) {
        Toast.error('加载用户列表失败: ' + err.message);
    }
}

async function changeMyPassword() {
    const oldPwd = document.getElementById('old-pwd').value;
    const newPwd = document.getElementById('new-pwd').value;
    const confirmPwd = document.getElementById('confirm-pwd').value;

    if (!oldPwd || !newPwd) {
        Toast.error('请填写完整');
        return;
    }
    if (newPwd !== confirmPwd) {
        Toast.error('两次密码不一致');
        return;
    }
    if (newPwd.length < 6) {
        Toast.error('密码至少6位');
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
    }, '确认', '确认删除');
}
