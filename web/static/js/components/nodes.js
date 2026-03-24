// Nodes Component
function renderNodes() {
    const content = document.getElementById('page-content');
    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>节点管理</h2>
                    <p class="subtitle">管理所有转发节点</p>
                </div>
                <button class="btn btn-primary" onclick="showCreateNodeModal()">+ 添加节点</button>
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
                <td>${n.cpu_usage.toFixed(1)}%</td>
                <td>${formatNodeMemory(n)}</td>
                <td>
                    <div class="action-group">
                        <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${n.id})" title="详情">📋</button>
                        <button class="btn btn-sm btn-secondary" onclick="showDeployCmd(${n.id}, '${escHTML(n.api_key)}')" title="部署命令">🔗</button>
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteNode(${n.id}, '${escHTML(n.name)}')" title="删除">🗑</button>
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
                        <span class="m-card-val">${n.cpu_usage.toFixed(1)}%</span>
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
                        <button class="btn btn-sm btn-secondary" onclick="showDeployCmd(${n.id}, '${escHTML(n.api_key)}')">部署</button>
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteNode(${n.id}, '${escHTML(n.name)}')">删除</button>
                    </div>
                </div>
            </div>
        `).join('');
    } catch (err) {
        Toast.error('加载节点失败: ' + err.message);
    }
}

function getNodePanelWSURL() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    return `${wsProtocol}://${window.location.host}/ws/node`;
}

function showCreateNodeModal() {
    showModal('添加节点', `
        <div class="form-group">
            <label>节点名称</label>
            <input type="text" class="form-input" id="node-name" placeholder="例如: HK-Node-1" autofocus>
        </div>
        <div class="form-group">
            <label>分组 (可选)</label>
            <input type="text" class="form-input" id="node-group" placeholder="例如: 香港">
        </div>
    `, async () => {
        const name = document.getElementById('node-name').value.trim();
        if (!name) { Toast.error('请输入节点名称'); return; }

        try {
            const res = await API.createNode(name, document.getElementById('node-group').value.trim());
            closeModal();
            Toast.success('节点创建成功');

            // Show deploy command
            const node = res.node;
            const wsURL = getNodePanelWSURL();
            showModal('节点部署', `
                <p style="color:var(--text-secondary);margin-bottom:16px;">在目标服务器上执行以下命令部署节点：</p>
                <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">
./flowgate node --panel ${wsURL} --key ${node.api_key}
                </div>
                <p style="color:var(--text-muted);font-size:0.78rem;margin-top:12px;">💡 点击命令可复制</p>
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

function showDeployCmd(id, apiKey) {
    const wsURL = getNodePanelWSURL();
    showModal('部署命令', `
        <p style="color:var(--text-secondary);margin-bottom:16px;">在目标服务器上执行以下命令：</p>
        <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">
./flowgate node --panel ${wsURL} --key ${apiKey}
        </div>
        <p style="color:var(--text-muted);font-size:0.78rem;margin-top:12px;">💡 点击命令可复制</p>
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
        } catch (err) {
            Toast.error('删除失败: ' + err.message);
        }
    }, '确认', '确认删除');
}
