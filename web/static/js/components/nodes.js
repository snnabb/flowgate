// Nodes Component
let nodeGroupCache = [];
let _nodeOwnerUsers = [];
let _nodeOwnerUserMap = {};

function renderNodes() {
    const content = document.getElementById('page-content');
    const currentUser = API.getUser();
    const isManager = currentUser && isManagerRole(currentUser.role);
    const isAdmin = currentUser && isAdminRole(currentUser.role);

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>节点管理</h2>
                    <p class="subtitle">管理可见转发节点及其归属账号</p>
                </div>
                <div style="display:flex;gap:8px;flex-wrap:wrap;">
                    ${isAdmin ? '<button class="btn btn-secondary" onclick="showNodeGroupsModal()">分组管理</button>' : ''}
                    ${isManager ? '<button class="btn btn-primary" onclick="showCreateNodeModal()">+ 添加节点</button>' : ''}
                </div>
            </div>

            <div class="table-container desktop-only">
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>节点名称</th>
                            <th>归属用户</th>
                            <th>分组</th>
                            <th>状态</th>
                            <th>IP 地址</th>
                            <th>CPU</th>
                            <th>内存</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="nodes-body">
                        <tr><td colspan="9" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only m-card-list" id="nodes-cards">
                <p style="color:var(--text-muted);text-align:center;padding:20px;">加载中...</p>
            </div>
        </div>
    `;

    loadNodes();
    if (isAdmin) {
        loadNodeGroups();
    }
}

async function loadNodes() {
    try {
        const currentUser = API.getUser();
        const isManager = currentUser && isManagerRole(currentUser.role);
        const [nodeRes, usersRes] = await Promise.all([
            API.getNodes(),
            isManager ? API.getUsers().catch(() => ({ users: [] })) : Promise.resolve({ users: [] }),
        ]);

        const nodes = nodeRes.nodes || [];
        _nodeOwnerUsers = usersRes.users || [];
        _nodeOwnerUserMap = buildUserMap(_nodeOwnerUsers);

        const body = document.getElementById('nodes-body');
        const cards = document.getElementById('nodes-cards');

        if (nodes.length === 0) {
            if (body) body.innerHTML = '<tr><td colspan="9" class="empty-state"><p>暂无节点</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">暂无节点</p>';
            return;
        }

        if (body) body.innerHTML = nodes.map(node => `
            <tr>
                <td>#${node.id}</td>
                <td><strong>${escHTML(node.name)}</strong></td>
                <td>${escHTML(resolveUserLabel(node.owner_user_id, _nodeOwnerUserMap, currentUser))}</td>
                <td>${escHTML(node.group_name) || '-'}</td>
                <td><span class="badge badge-${node.status}"><span class="badge-dot"></span>${node.status === 'online' ? '在线' : '离线'}</span></td>
                <td>${escHTML(node.ip_addr) || '-'}</td>
                <td>${Number(node.cpu_usage || 0).toFixed(1)}%</td>
                <td>${formatNodeMemory(node)}</td>
                <td>${renderNodeActions(node, isManager)}</td>
            </tr>
        `).join('');

        if (cards) cards.innerHTML = nodes.map(node => `
            <div class="m-card">
                <div class="m-card-head">
                    <span class="m-card-title">${escHTML(node.name)}</span>
                    <span class="badge badge-${node.status}"><span class="badge-dot"></span>${node.status === 'online' ? '在线' : '离线'}</span>
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">归属用户</span>
                        <span class="m-card-val">${escHTML(resolveUserLabel(node.owner_user_id, _nodeOwnerUserMap, currentUser))}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">分组</span>
                        <span class="m-card-val">${escHTML(node.group_name) || '-'}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">IP</span>
                        <span class="m-card-val">${escHTML(node.ip_addr) || '-'}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">CPU</span>
                        <span class="m-card-val">${Number(node.cpu_usage || 0).toFixed(1)}%</span>
                    </div>
                    <div class="m-card-row full">
                        <span class="m-card-label">内存</span>
                        <span class="m-card-val">${formatNodeMemory(node)}</span>
                    </div>
                </div>
                <div class="m-card-foot">
                    <span class="m-card-id">#${node.id}</span>
                    <div class="action-group" style="display:flex;gap:6px;">
                        <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${node.id})">规则</button>
                        ${isManager ? `<button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${node.id}, ${JSON.stringify(node.api_key || "")})'>部署</button>` : ''}
                        ${isManager ? `<button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${node.id}, ${JSON.stringify(node.name || "")})'>删除</button>` : ''}
                    </div>
                </div>
            </div>
        `).join('');
    } catch (err) {
        Toast.error('加载节点失败: ' + err.message);
    }
}

function renderNodeActions(node, isManager) {
    return `
        <div class="action-group">
            <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${node.id})" title="详情">详情</button>
            ${isManager ? `<button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${node.id}, ${JSON.stringify(node.api_key || "")})' title="部署命令">部署</button>` : ''}
            ${isManager ? `<button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${node.id}, ${JSON.stringify(node.name || "")})' title="删除">删除</button>` : ''}
        </div>
    `;
}

async function loadNodeGroups() {
    try {
        const res = await API.getNodeGroups();
        nodeGroupCache = res.node_groups || [];
    } catch {
        nodeGroupCache = [];
    }
    return nodeGroupCache;
}

async function loadVisibleNodeOwners() {
    const currentUser = API.getUser();
    if (!currentUser || !isManagerRole(currentUser.role)) {
        return [];
    }

    try {
        const res = await API.getUsers();
        _nodeOwnerUsers = res.users || [];
        _nodeOwnerUserMap = buildUserMap(_nodeOwnerUsers);
    } catch {
        _nodeOwnerUsers = [currentUser];
        _nodeOwnerUserMap = buildUserMap(_nodeOwnerUsers);
    }
    return _nodeOwnerUsers;
}

function getNodePanelWSURL() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    return `${wsProtocol}://${window.location.host}/ws/node`;
}

function getInstallURL(apiKey) {
    return `${window.location.origin}/api/node/install/${apiKey}`;
}

function renderNodeGroupOptions() {
    return (nodeGroupCache || [])
        .map(group => `<option value="${escHTML(group.name)}"></option>`)
        .join('');
}

function renderNodeOwnerOptions(users) {
    return (users || [])
        .map(user => `<option value="${user.id}">${escHTML(user.username)} (${user.role})</option>`)
        .join('');
}

function renderNodeGroupList() {
    if (!nodeGroupCache || nodeGroupCache.length === 0) {
        return '<p style="color:var(--text-muted);font-size:0.9rem;">暂无节点分组，可在下方创建。</p>';
    }

    return `
        <div style="display:flex;flex-direction:column;gap:10px;">
            ${nodeGroupCache.map(group => `
                <div style="border:1px solid var(--border-color);border-radius:12px;padding:12px;display:flex;justify-content:space-between;align-items:flex-start;gap:12px;">
                    <div style="min-width:0;">
                        <div style="font-weight:600;">${escHTML(group.name)}</div>
                        <div style="color:var(--text-muted);font-size:0.85rem;margin-top:4px;">${escHTML(group.description || '暂无描述')}</div>
                        <div style="color:var(--text-muted);font-size:0.8rem;margin-top:6px;">节点数量: ${Number(group.node_count || 0)}</div>
                    </div>
                    <button class="btn btn-sm btn-danger" onclick='confirmDeleteNodeGroup(${group.id}, ${JSON.stringify(group.name)})'>删除</button>
                </div>
            `).join('')}
        </div>
    `;
}

async function showCreateNodeModal() {
    const currentUser = API.getUser();
    if (!currentUser || !isManagerRole(currentUser.role)) {
        Toast.error('当前账号没有创建节点的权限');
        return;
    }

    const [groups, owners] = await Promise.all([loadNodeGroups(), loadVisibleNodeOwners()]);
    const defaultOwner = owners.find(user => user.id === currentUser.id)?.id || (owners[0] ? owners[0].id : currentUser.id);

    showModal('添加节点', `
        <div class="form-group">
            <label>节点名称</label>
            <input type="text" class="form-input" id="node-name" placeholder="例如: HK-Node-1" autofocus>
        </div>
        <div class="form-row">
            <div class="form-group">
                <label>归属用户</label>
                <select class="form-select" id="node-owner">
                    ${renderNodeOwnerOptions(owners)}
                </select>
            </div>
            <div class="form-group">
                <label>分组 (可选)</label>
                <input type="text" class="form-input" id="node-group" list="node-group-options" placeholder="例如: 香港入口组">
                <datalist id="node-group-options">${renderNodeGroupOptions(groups)}</datalist>
            </div>
        </div>
        <div id="node-owner-filtered-users" class="mini-meta">
            owner 仅可选当前账号可见的用户范围。
        </div>
    `, async () => {
        const name = document.getElementById('node-name').value.trim();
        const groupName = document.getElementById('node-group').value.trim();
        const ownerId = parseInt(document.getElementById('node-owner').value || defaultOwner, 10);

        if (!name) {
            Toast.error('请输入节点名称');
            return;
        }

        try {
            const res = await API.createNode({
                name,
                group_name: groupName,
                owner_user_id: ownerId,
            });
            closeModal();
            Toast.success('节点创建成功');

            const node = res.node;
            const installURL = getInstallURL(node.api_key);
            const wsURL = getNodePanelWSURL();
            showModal('节点部署', `
                <p style="color:var(--text-secondary);margin-bottom:12px;font-weight:500;">一键部署（推荐）</p>
                <p style="color:var(--text-muted);font-size:0.82rem;margin-bottom:8px;">在目标 Linux 服务器上以 root 执行：</p>
                <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">
curl -sSL ${installURL} | bash
                </div>
                <p style="color:var(--text-muted);font-size:0.75rem;margin-top:8px;">自动下载、安装、配置 systemd 服务并启动。</p>
                <details style="margin-top:16px;">
                    <summary style="color:var(--text-secondary);cursor:pointer;font-size:0.85rem;">手动部署</summary>
                    <div style="margin-top:10px;">
                        <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">
./flowgate node --panel ${wsURL} --key ${node.api_key}
                        </div>
                    </div>
                </details>
                <div class="form-group" style="margin-top:16px;">
                    <label>API Key</label>
                    <div class="copy-text" onclick="copyToClipboard('${node.api_key}')" style="max-width:100%">${node.api_key}</div>
                </div>
            `, null, '关闭');

            loadNodes();
        } catch (err) {
            Toast.error('创建失败: ' + err.message);
        }
    });
}

async function showNodeGroupsModal() {
    const currentUser = API.getUser();
    if (!currentUser || !isAdminRole(currentUser.role)) {
        Toast.error('只有 admin 可以管理节点分组');
        return;
    }

    await loadNodeGroups();

    showModal('节点分组', `
        <div style="display:flex;flex-direction:column;gap:16px;">
            <div>
                <div style="font-weight:600;margin-bottom:10px;">现有分组</div>
                ${renderNodeGroupList()}
            </div>
            <div style="border-top:1px solid var(--border-color);padding-top:16px;">
                <div style="font-weight:600;margin-bottom:10px;">创建新分组</div>
                <div class="form-group">
                    <label>分组名称</label>
                    <input type="text" class="form-input" id="node-group-name" placeholder="例如: entry-hk" autofocus>
                </div>
                <div class="form-group">
                    <label>描述 (可选)</label>
                    <input type="text" class="form-input" id="node-group-description" placeholder="例如: 香港入口节点">
                </div>
            </div>
        </div>
    `, createNodeGroupFromModal, '关闭', '创建分组');
}

async function createNodeGroupFromModal() {
    const name = document.getElementById('node-group-name')?.value.trim() || '';
    const description = document.getElementById('node-group-description')?.value.trim() || '';

    if (!name) {
        Toast.error('请输入分组名称');
        return;
    }

    try {
        await API.createNodeGroup(name, description);
        Toast.success('分组创建成功');
        await showNodeGroupsModal();
    } catch (err) {
        Toast.error('创建分组失败: ' + err.message);
    }
}

async function confirmDeleteNodeGroup(id, name) {
    showModal('删除节点分组', `
        <p style="color:var(--color-danger);">确定要删除分组 <strong>${escHTML(name)}</strong> 吗？</p>
        <p style="color:var(--text-muted);font-size:0.85rem;margin-top:8px;">若该分组仍被节点使用，后端会拒绝删除。</p>
    `, async () => {
        try {
            await API.deleteNodeGroup(id);
            Toast.success('分组已删除');
            await showNodeGroupsModal();
            loadNodes();
        } catch (err) {
            Toast.error('删除分组失败: ' + err.message);
        }
    }, '取消', '确认删除');
}

function showDeployCmd(id, apiKey) {
    const wsURL = getNodePanelWSURL();
    const installURL = getInstallURL(apiKey);
    showModal('部署命令', `
        <p style="color:var(--text-secondary);margin-bottom:12px;font-weight:500;">一键部署（推荐）</p>
        <p style="color:var(--text-muted);font-size:0.82rem;margin-bottom:8px;">在目标 Linux 服务器上以 root 执行：</p>
        <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">
curl -sSL ${installURL} | bash
        </div>
        <p style="color:var(--text-muted);font-size:0.75rem;margin-top:8px;">自动下载、安装、配置 systemd 服务并启动。</p>
        <details style="margin-top:16px;">
            <summary style="color:var(--text-secondary);cursor:pointer;font-size:0.85rem;">手动部署</summary>
            <div style="margin-top:10px;">
                <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">
./flowgate node --panel ${wsURL} --key ${apiKey}
                </div>
            </div>
        </details>
        <div class="form-group" style="margin-top:16px;">
            <label>API Key</label>
            <div class="copy-text" onclick="copyToClipboard('${apiKey}')" style="max-width:100%">${apiKey}</div>
        </div>
    `, null, '关闭');
}

function showNodeDetail(id) {
    Router.navigate(`/rules?node_id=${id}`);
}

async function confirmDeleteNode(id, name) {
    showModal('删除节点', `
        <p style="color:var(--color-danger);">确定要删除节点 <strong>${escHTML(name)}</strong> 吗？</p>
        <p style="color:var(--text-muted);font-size:0.85rem;margin-top:8px;">此操作将同时删除该节点下的所有转发规则。</p>
    `, async () => {
        try {
            await API.deleteNode(id);
            closeModal();
            Toast.success('节点已删除');
            loadNodes();
            loadNodeGroups();
        } catch (err) {
            Toast.error('删除失败: ' + err.message);
        }
    }, '取消', '确认删除');
}
