// Nodes Component
let nodeGroupCache = [];

function renderNodes() {
    const content = document.getElementById('page-content');
    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>节点管理</h2>
                    <p class="subtitle">管理所有转发节点</p>
                </div>
                <div style="display:flex;gap:8px;flex-wrap:wrap;">
                    <button class="btn btn-secondary" onclick="showNodeGroupsModal()">分组管理</button>
                    <button class="btn btn-primary" onclick="showCreateNodeModal()">+ 添加节点</button>
                </div>
            </div>

            <div class="table-container desktop-only">
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>节点名称</th>
                            <th>分组</th>
                            <th>状态</th>
                            <th>IP 地址</th>
                            <th>CPU</th>
                            <th>内存</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="nodes-body">
                        <tr><td colspan="8" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only m-card-list" id="nodes-cards">
                <p style="color:var(--text-muted);text-align:center;padding:20px;">加载中...</p>
            </div>
        </div>
    `;

    loadNodes();
    loadNodeGroups();
}

async function loadNodes() {
    try {
        const res = await API.getNodes();
        const nodes = res.nodes || [];
        const body = document.getElementById('nodes-body');
        const cards = document.getElementById('nodes-cards');

        if (nodes.length === 0) {
            if (body) body.innerHTML = '<tr><td colspan="8" class="empty-state"><p>暂无节点，点击上方按钮添加</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">暂无节点，点击上方按钮添加</p>';
            return;
        }

        if (body) body.innerHTML = nodes.map(n => `
            <tr>
                <td>#${n.id}</td>
                <td><strong>${escHTML(n.name)}</strong></td>
                <td>${escHTML(n.group_name) || '-'}</td>
                <td><span class="badge badge-${n.status}"><span class="badge-dot"></span>${n.status === 'online' ? '在线' : '离线'}</span></td>
                <td>${escHTML(n.ip_addr) || '-'}</td>
                <td>${Number(n.cpu_usage || 0).toFixed(1)}%</td>
                <td>${formatNodeMemory(n)}</td>
                <td>
                    <div class="action-group">
                        <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${n.id})" title="详情">详情</button>
                        <button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${n.id}, ${JSON.stringify(n.api_key || "")})' title="部署命令">部署</button>
                        <button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${n.id}, ${JSON.stringify(n.name || "")})' title="删除">删除</button>
                    </div>
                </td>
            </tr>
        `).join('');

        if (cards) cards.innerHTML = nodes.map(n => `
            <div class="m-card">
                <div class="m-card-head">
                    <span class="m-card-title">${escHTML(n.name)}</span>
                    <span class="badge badge-${n.status}"><span class="badge-dot"></span>${n.status === 'online' ? '在线' : '离线'}</span>
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">分组</span>
                        <span class="m-card-val">${escHTML(n.group_name) || '-'}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">IP</span>
                        <span class="m-card-val">${escHTML(n.ip_addr) || '-'}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">CPU</span>
                        <span class="m-card-val">${Number(n.cpu_usage || 0).toFixed(1)}%</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">内存</span>
                        <span class="m-card-val">${formatNodeMemory(n)}</span>
                    </div>
                </div>
                <div class="m-card-foot">
                    <span class="m-card-id">#${n.id}</span>
                    <div class="action-group" style="display:flex;gap:6px;">
                        <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${n.id})">规则</button>
                        <button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${n.id}, ${JSON.stringify(n.api_key || "")})'>部署</button>
                        <button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${n.id}, ${JSON.stringify(n.name || "")})'>删除</button>
                    </div>
                </div>
            </div>
        `).join('');
    } catch (err) {
        Toast.error('加载节点失败: ' + err.message);
    }
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
    await loadNodeGroups();

    showModal('添加节点', `
        <div class="form-group">
            <label>节点名称</label>
            <input type="text" class="form-input" id="node-name" placeholder="例如: HK-Node-1" autofocus>
        </div>
        <div class="form-group">
            <label>分组 (可选)</label>
            <input type="text" class="form-input" id="node-group" list="node-group-options" placeholder="例如: 香港入口组">
            <datalist id="node-group-options">${renderNodeGroupOptions()}</datalist>
        </div>
    `, async () => {
        const name = document.getElementById('node-name').value.trim();
        const groupName = document.getElementById('node-group').value.trim();
        if (!name) {
            Toast.error('请输入节点名称');
            return;
        }

        try {
            const res = await API.createNode(name, groupName);
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
                <p style="color:var(--text-muted);font-size:0.72rem;margin-top:4px;">点击命令或 Key 可复制</p>
            `, null, '关闭');

            loadNodes();
        } catch (err) {
            Toast.error('创建失败: ' + err.message);
        }
    });
}

async function showNodeGroupsModal() {
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
                <p style="color:var(--text-muted);font-size:0.82rem;margin-bottom:8px;">手动运行节点：</p>
                <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">
./flowgate node --panel ${wsURL} --key ${apiKey}
                </div>
            </div>
        </details>
        <div class="form-group" style="margin-top:16px;">
            <label>API Key</label>
            <div class="copy-text" onclick="copyToClipboard('${apiKey}')" style="max-width:100%">${apiKey}</div>
        </div>
        <p style="color:var(--text-muted);font-size:0.72rem;margin-top:4px;">点击命令或 Key 可复制</p>
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
