function renderNodes() {
    const content = document.getElementById('page-content');
    const currentUser = API.getUser();
    const canManage = currentUser && isManagerRole(currentUser.role);

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>Nodes</h2>
                    <p class="subtitle">${canManage ? 'Manage the shared node pool' : 'View nodes assigned to your account'}</p>
                </div>
                ${canManage ? '<button class="btn btn-primary" onclick="showCreateNodeModal()">+ Add Node</button>' : ''}
            </div>

            <div class="table-container desktop-only">
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>Name</th>
                            <th>Status</th>
                            <th>IP</th>
                            <th>CPU</th>
                            <th>Memory</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody id="nodes-body">
                        <tr><td colspan="7" class="empty-state"><p>Loading...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only m-card-list" id="nodes-cards">
                <p style="color:var(--text-muted);text-align:center;padding:20px;">Loading...</p>
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
            if (body) body.innerHTML = '<tr><td colspan="7" class="empty-state"><p>No nodes</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">No nodes</p>';
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
                            <span class="m-card-label">Memory</span>
                            <span class="m-card-val">${formatNodeMemory(node)}</span>
                        </div>
                    </div>
                    <div class="m-card-foot">
                        <span class="m-card-id">#${node.id}</span>
                        <div class="action-group" style="display:flex;gap:6px;">
                            <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${node.id})">Details</button>
                            ${canManage ? `<button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${node.id}, ${JSON.stringify(node.api_key || '')})'>Deploy</button>` : ''}
                            ${canManage ? `<button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${node.id}, ${JSON.stringify(node.name || '')})'>Delete</button>` : ''}
                        </div>
                    </div>
                </div>
            `).join('');
        }
    } catch (error) {
        Toast.error(`Failed to load nodes: ${error.message}`);
    }
}

function renderNodeActions(node, canManage) {
    return `
        <div class="action-group">
            <button class="btn btn-sm btn-secondary" onclick="showNodeDetail(${node.id})">Details</button>
            ${canManage ? `<button class="btn btn-sm btn-secondary" onclick='showDeployCmd(${node.id}, ${JSON.stringify(node.api_key || '')})'>Deploy</button>` : ''}
            ${canManage ? `<button class="btn btn-sm btn-danger" onclick='confirmDeleteNode(${node.id}, ${JSON.stringify(node.name || '')})'>Delete</button>` : ''}
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
        'Add Node',
        `
            <div class="form-group">
                <label>Node name</label>
                <input type="text" class="form-input" id="node-name" placeholder="For example: hk-edge-01" autofocus>
            </div>
        `,
        async () => {
            const name = document.getElementById('node-name').value.trim();
            if (!name) {
                Toast.error('Node name is required');
                return;
            }

            try {
                const response = await API.createNode({ name });
                closeModal();
                Toast.success('Node created');
                showDeployCmd(response.node.id, response.node.api_key || '');
                loadNodes();
            } catch (error) {
                Toast.error(`Failed to create node: ${error.message}`);
            }
        },
        'Cancel',
        'Create',
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
            `Node ${node.name}`,
            `
                <div style="display:grid;gap:12px;">
                    <div class="mini-meta">Status: ${escHTML(node.status)} | IP: ${escHTML(node.ip_addr || '-')}</div>
                    <div class="mini-meta">CPU: ${Number(node.cpu_usage || 0).toFixed(1)}% | Memory: ${formatNodeMemory(node)}</div>
                    <div style="border-top:1px solid var(--border-color);padding-top:12px;">
                        <div style="font-weight:600;margin-bottom:8px;">Rules on this node</div>
                        ${rules.length ? `
                            <div style="display:flex;flex-direction:column;gap:8px;">
                                ${rules.map((rule) => `
                                    <div style="border:1px solid var(--border-color);border-radius:10px;padding:10px;">
                                        <div style="display:flex;justify-content:space-between;gap:12px;">
                                            <strong>${escHTML(rule.name || `Rule #${rule.id}`)}</strong>
                                            <span>${formatBandwidthLimit(rule.speed_limit)}</span>
                                        </div>
                                        <div class="mini-meta" style="margin-top:6px;">${escHTML(rule.protocol.toUpperCase())} :${rule.listen_port} -> ${escHTML(rule.target_addr)}:${rule.target_port}</div>
                                    </div>
                                `).join('')}
                            </div>
                        ` : '<p style="color:var(--text-muted);">No rules on this node</p>'}
                    </div>
                </div>
            `,
            null,
            'Close',
            null,
        );
    } catch (error) {
        Toast.error(`Failed to load node details: ${error.message}`);
    }
}

function showDeployCmd(id, apiKey) {
    const wsURL = getNodePanelWSURL();
    const installURL = getInstallURL(apiKey);

    showModal(
        'Node Deploy',
        `
            <p style="color:var(--text-secondary);margin-bottom:12px;font-weight:500;">Recommended one-line install</p>
            <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">curl -sSL ${installURL} | bash</div>
            <p style="color:var(--text-muted);font-size:0.75rem;margin-top:8px;">This downloads the binary, installs a service, and starts the node.</p>
            <details style="margin-top:16px;">
                <summary style="color:var(--text-secondary);cursor:pointer;font-size:0.85rem;">Manual command</summary>
                <div style="margin-top:10px;">
                    <div class="deploy-cmd" onclick="copyToClipboard(this.textContent.trim())">./flowgate node --panel ${wsURL} --key ${apiKey}</div>
                </div>
            </details>
            <div class="form-group" style="margin-top:16px;">
                <label>API Key</label>
                <div class="copy-text" onclick="copyToClipboard('${apiKey}')" style="max-width:100%">${apiKey}</div>
            </div>
            <div class="mini-meta">Node #${id}</div>
        `,
        null,
        'Close',
        null,
    );
}

function confirmDeleteNode(id, name) {
    showModal(
        'Delete Node',
        `<p style="color:var(--color-danger);">Delete <strong>${escHTML(name)}</strong>?</p>`,
        async () => {
            try {
                await API.deleteNode(id);
                closeModal();
                Toast.success('Node deleted');
                loadNodes();
            } catch (error) {
                Toast.error(`Failed to delete node: ${error.message}`);
            }
        },
        'Cancel',
        'Delete',
    );
}
