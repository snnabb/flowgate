function renderNodes() {
    const content = document.getElementById('page-content');
    const currentUser = API.getUser();
    const canManage = currentUser && isManagerRole(currentUser.role);

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>节点管理</h2>
                    <p class="subtitle">${canManage ? '管理共享节点资源池' : '查看已分配给你的节点'}</p>
                </div>
                ${canManage ? '<button class="btn btn-primary" onclick="showCreateNodeModal()">+ 添加节点</button>' : ''}
            </div>

            <div class="table-container desktop-only">
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>名称</th>
                            <th>状态</th>
                            <th>IP</th>
                            <th>CPU</th>
                            <th>内存</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="nodes-body">
                        <tr><td colspan="7" class="empty-state"><p>加载中...</p></td></tr>
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
        const currentUser = API.getUser();
        const canManage = currentUser && isManagerRole(currentUser.role);
        const response = await API.getNodes();
        const nodes = response.nodes || [];
        const body = document.getElementById('nodes-body');
        const cards = document.getElementById('nodes-cards');

        if (!nodes.length) {
            if (body) body.innerHTML = '<tr><td colspan="7" class="empty-state"><p>暂无节点</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">暂无节点</p>';
            return;
        }

        if (body) {
            body.innerHTML = nodes.map((node) => `
                <tr>
                    <td>#${node.id}</td>
                    <td><strong>${escHTML(node.name)}</strong></td>
                    <td><span class="badge badge-${node.status}"><span class="badge-dot"></span>${node.status}</span></td>
                    <td>${escHTML(node.ip_addr || '-')}</td>
                    <td>${Number(node.cpu_usage || 0).toFixed(1)}%</td>
                    <td>${formatNodeMemory(node)}</td>
                    <td>${renderNodeActions(node, canManage)}</td>
                </tr>
            `).join('');
        }

        if (cards) {
            cards.innerHTML = nodes.map((node) => `
                <div class="m-card">
                    <div class="m-card-head">
                        <span class="m-card-title">${escHTML(node.name)}</span>
                        <span class="badge badge-${node.status}"><span class="badge-dot"></span>${node.status}</span>
                    </div>
                    <div class="m-card-body">
                        <div class="m-card-row">
                            <span class="m-card-label">IP</span>
                            <span class="m-card-val">${escHTML(node.ip_addr || '-')}</span>
                        </div>
                        <div class="m-card-row">
                            <span class="m-card-label">CPU</span>
                            <span class="m-card-val">${Number(node.cpu_usage || 0).toFixed(1)}%</span>
                        </div>
                        <div class="m-card-row">
                            <span class="m-card-label">内存</span>
                            <span class="m-card-val">${formatNodeMemory(node)}</span>
                        </div>
                    </div>
                    <div class="m-card-foot">
                        <span class="m-card-id">#${node.id}</span>
                        <div class="action-group" style="display:flex;gap:6px;">
                            <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${node.id})">详情</button>
                            ${canManage ? `<button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${node.id}, ${JSON.stringify(node.api_key || '')})'>部署</button>` : ''}
                            ${canManage ? `<button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${node.id}, ${JSON.stringify(node.name || '')})'>删除</button>` : ''}
                        </div>
                    </div>
                </div>
            `).join('');
        }
    } catch (error) {
        Toast.error(`加载节点失败：${error.message}`);
    }
}

function renderNodeActions(node, canManage) {
    return `
        <div class="action-group">
            <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${node.id})">详情</button>
            ${canManage ? `<button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${node.id}, ${JSON.stringify(node.api_key || '')})'>部署</button>` : ''}
            ${canManage ? `<button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${node.id}, ${JSON.stringify(node.name || '')})'>删除</button>` : ''}
        </div>
    `;
}

function getNodePanelWSURL() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    return `${wsProtocol}://${window.location.host}/ws/node`;
}

function getInstallURL(apiKey) {
    return `${window.location.origin}/api/node/install/${apiKey}`;
}

function showCreateNodeModal() {
    showModal(
        '添加节点',
        `
            <div class="form-group">
                <label>节点名称</label>
                <input type="text" class="form-input" id="node-name" placeholder="例如：hk-edge-01" autofocus>
            </div>
        `,
        async () => {
            const name = document.getElementById('node-name').value.trim();
            if (!name) {
                Toast.error('请输入节点名称');
                return;
            }

            try {
                const response = await API.createNode({ name });
                closeModal();
                Toast.success('节点已创建');
                showDeployCmd(response.node.id, response.node.api_key || '');
                loadNodes();
            } catch (error) {
                Toast.error(`创建节点失败：${error.message}`);
            }
        },
        '取消',
        '创建',
    );
}

async function showNodeDetail(id) {
    try {
        const [nodeRes, rulesRes] = await Promise.all([
            API.getNode(id),
            API.getRules(id),
        ]);
        const node = nodeRes.node;
        const rules = rulesRes.rules || [];

        showModal(
            `节点 ${node.name}`,
            `
                <div style="display:grid;gap:12px;">
                    <div class="mini-meta">状态：${escHTML(node.status)} | IP：${escHTML(node.ip_addr || '-')}</div>
                    <div class="mini-meta">CPU：${Number(node.cpu_usage || 0).toFixed(1)}% | 内存：${formatNodeMemory(node)}</div>
                    <div style="border-top:1px solid var(--border-color);padding-top:12px;">
                        <div style="font-weight:600;margin-bottom:8px;">本节点规则</div>
                        ${rules.length ? `
                            <div style="display:flex;flex-direction:column;gap:8px;">
                                ${rules.map((rule) => `
                                    <div style="border:1px solid var(--border-color);border-radius:10px;padding:10px;">
                                        <div style="display:flex;justify-content:space-between;gap:12px;">
                                            <strong>${escHTML(rule.name || `规则 #${rule.id}`)}</strong>
                                            <span>${formatBandwidthLimit(rule.speed_limit)}</span>
                                        </div>
                                        <div class="mini-meta" style="margin-top:6px;">${escHTML(rule.protocol.toUpperCase())} :${rule.listen_port} -> ${escHTML(rule.target_addr)}:${rule.target_port}</div>
                                    </div>
                                `).join('')}
                            </div>
                        ` : '<p style="color:var(--text-muted);">该节点暂无规则</p>'}
                    </div>
                </div>
            `,
            null,
            '关闭',
            null,
        );
    } catch (error) {
        Toast.error(`加载节点详情失败：${error.message}`);
    }
}

function showDeployCmd(id, apiKey) {
    const wsURL = getNodePanelWSURL();
    const installURL = getInstallURL(apiKey);

    showModal(
        '节点部署',
        `
            <p style="color:var(--text-secondary);margin-bottom:12px;font-weight:500;">推荐一键安装命令</p>
            <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">curl -sSL ${installURL} | bash</div>
            <p style="color:var(--text-muted);font-size:0.75rem;margin-top:8px;">这条命令会下载二进制、安装服务并启动节点。</p>
            <details style="margin-top:16px;">
                <summary style="color:var(--text-secondary);cursor:pointer;font-size:0.85rem;">手动命令</summary>
                <div style="margin-top:10px;">
                    <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">./flowgate node --panel ${wsURL} --key ${apiKey}</div>
                </div>
            </details>
            <div class="form-group" style="margin-top:16px;">
                <label>API Key</label>
                <div class="copy-text" onclick="copyToClipboard('${apiKey}')" style="max-width:100%">${apiKey}</div>
            </div>
            <div class="mini-meta">节点 #${id}</div>
        `,
        null,
        '关闭',
        null,
    );
}

function confirmDeleteNode(id, name) {
    showModal(
        '删除节点',
        `<p style="color:var(--color-danger);">确认删除 <strong>${escHTML(name)}</strong>？</p>`,
        async () => {
            try {
                await API.deleteNode(id);
                closeModal();
                Toast.success('节点已删除');
                loadNodes();
            } catch (error) {
                Toast.error(`删除节点失败：${error.message}`);
            }
        },
        '取消',
        '删除',
    );
}
