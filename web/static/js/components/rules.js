// Rules Component
function renderRules() {
    const content = document.getElementById('page-content');
    const params = new URLSearchParams(window.location.search);
    const nodeFilter = params.get('node_id') || '';

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>转发规则</h2>
                    <p class="subtitle">管理端口转发规则与节点执行状态</p>
                </div>
                <div style="display:flex;gap:8px;align-items:center;">
                    <select class="form-select" id="rule-node-filter" style="width:180px;" onchange="filterRulesByNode()">
                        <option value="">全部节点</option>
                    </select>
                    <button class="btn btn-primary" onclick="showCreateRuleModal()">+ 添加规则</button>
                </div>
            </div>

            <div class="table-container">
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>名称</th>
                            <th>节点</th>
                            <th>协议</th>
                            <th>监听端口</th>
                            <th>目标地址</th>
                            <th>限速</th>
                            <th>入站流量</th>
                            <th>出站流量</th>
                            <th>状态</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="rules-body">
                        <tr><td colspan="11" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
        </div>
    `;

    loadNodeOptions(nodeFilter);
    loadRules(nodeFilter ? parseInt(nodeFilter, 10) : 0);
}

async function loadNodeOptions(selectedId) {
    try {
        const res = await API.getNodes();
        const select = document.getElementById('rule-node-filter');
        (res.nodes || []).forEach(node => {
            const option = document.createElement('option');
            option.value = node.id;
            option.textContent = node.name;
            if (String(node.id) === selectedId) {
                option.selected = true;
            }
            select.appendChild(option);
        });
    } catch (err) {
        Toast.error('加载节点列表失败: ' + err.message);
    }
}

function filterRulesByNode() {
    const nodeId = document.getElementById('rule-node-filter').value;
    const url = nodeId ? `/rules?node_id=${nodeId}` : '/rules';
    window.history.replaceState({}, '', url);
    loadRules(nodeId ? parseInt(nodeId, 10) : 0);
}

let _nodesCache = {};

function getRuleRuntimeMeta(rule) {
    switch (rule.runtime_status) {
        case 'running':
            return { badge: 'running', label: '运行中', note: rule.runtime_message || '节点已确认规则生效' };
        case 'error':
            return { badge: 'error', label: '失败', note: rule.runtime_message || '节点启动规则失败' };
        case 'offline':
            return { badge: 'offline', label: '待同步', note: rule.runtime_message || '节点离线，等待重连' };
        case 'stopped':
            return { badge: 'stopped', label: '已停用', note: rule.runtime_message || '规则当前未运行' };
        default:
            return { badge: 'pending', label: '待确认', note: rule.runtime_message || '规则已下发，等待节点确认' };
    }
}

function renderRuleStatus(rule) {
    const runtime = getRuleRuntimeMeta(rule);
    return `
        <div class="rule-status-cell">
            <span class="badge badge-${runtime.badge}"><span class="badge-dot"></span>${runtime.label}</span>
            <label class="toggle" title="${rule.enabled ? '启用' : '禁用'}">
                <input type="checkbox" ${rule.enabled ? 'checked' : ''} onchange="toggleRuleEnabled(${rule.id})">
                <span class="toggle-slider"></span>
            </label>
        </div>
        <div class="rule-status-note">${escHTML(runtime.note)}</div>
    `;
}

async function loadRules(nodeId) {
    try {
        const [rulesRes, nodesRes] = await Promise.all([
            API.getRules(nodeId || ''),
            API.getNodes()
        ]);

        _nodesCache = {};
        (nodesRes.nodes || []).forEach(node => {
            _nodesCache[node.id] = node.name;
        });

        const rules = rulesRes.rules || [];
        const body = document.getElementById('rules-body');

        if (rules.length === 0) {
            body.innerHTML = '<tr><td colspan="11" class="empty-state"><p>暂无转发规则</p></td></tr>';
            return;
        }

        body.innerHTML = rules.map(rule => {
            const protoClass = rule.protocol === 'tcp' ? 'tcp' : rule.protocol === 'udp' ? 'udp' : 'both';
            const speedText = rule.speed_limit > 0 ? `${rule.speed_limit} KB/s` : '无限';
            return `
                <tr>
                    <td>#${rule.id}</td>
                    <td>${escHTML(rule.name || `规则 #${rule.id}`)}</td>
                    <td>${escHTML(_nodesCache[rule.node_id] || `#${rule.node_id}`)}</td>
                    <td><span class="badge badge-${protoClass}">${rule.protocol.toUpperCase()}</span></td>
                    <td><strong>${rule.listen_port}</strong></td>
                    <td>${escHTML(rule.target_addr)}:${rule.target_port}</td>
                    <td>${speedText}</td>
                    <td>${formatBytes(rule.traffic_in)}</td>
                    <td>${formatBytes(rule.traffic_out)}</td>
                    <td>${renderRuleStatus(rule)}</td>
                    <td>
                        <div class="action-group">
                            <button class="btn btn-sm btn-secondary" onclick="showEditRuleModal(${rule.id})" title="编辑">✎</button>
                            <button class="btn btn-sm btn-danger" onclick="confirmDeleteRule(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')" title="删除">🗑</button>
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    } catch (err) {
        Toast.error('加载规则失败: ' + err.message);
    }
}

async function toggleRuleEnabled(id) {
    try {
        const res = await API.toggleRule(id);
        Toast.success(res.enabled ? '规则已启用' : '规则已禁用');
        const nodeId = document.getElementById('rule-node-filter')?.value || '';
        loadRules(nodeId ? parseInt(nodeId, 10) : 0);
    } catch (err) {
        Toast.error('操作失败: ' + err.message);
        const nodeId = document.getElementById('rule-node-filter')?.value || '';
        loadRules(nodeId ? parseInt(nodeId, 10) : 0);
    }
}

function showCreateRuleModal() {
    API.getNodes().then(res => {
        const nodes = res.nodes || [];
        if (nodes.length === 0) {
            Toast.error('请先创建并连接节点');
            return;
        }

        const nodeOptions = nodes.map(node => `<option value="${node.id}">${escHTML(node.name)}</option>`).join('');

        showModal('添加转发规则', `
            <div class="form-group">
                <label>规则名称</label>
                <input type="text" class="form-input" id="rule-name" placeholder="例如: Web 转发" autofocus>
            </div>
            <div class="form-group">
                <label>节点</label>
                <select class="form-select" id="rule-node">${nodeOptions}</select>
            </div>
            <div class="form-group">
                <label>协议</label>
                <select class="form-select" id="rule-protocol">
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                    <option value="tcp+udp">TCP+UDP</option>
                </select>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>监听端口</label>
                    <input type="number" class="form-input" id="rule-listen-port" placeholder="10000" min="1" max="65535">
                </div>
                <div class="form-group">
                    <label>限速 (KB/s, 0=无限)</label>
                    <input type="number" class="form-input" id="rule-speed" placeholder="0" min="0" value="0">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>目标地址</label>
                    <input type="text" class="form-input" id="rule-target-addr" placeholder="1.2.3.4">
                </div>
                <div class="form-group">
                    <label>目标端口</label>
                    <input type="number" class="form-input" id="rule-target-port" placeholder="443" min="1" max="65535">
                </div>
            </div>
        `, async () => {
            const rule = {
                name: document.getElementById('rule-name').value.trim(),
                node_id: parseInt(document.getElementById('rule-node').value, 10),
                protocol: document.getElementById('rule-protocol').value,
                listen_port: parseInt(document.getElementById('rule-listen-port').value, 10),
                target_addr: document.getElementById('rule-target-addr').value.trim(),
                target_port: parseInt(document.getElementById('rule-target-port').value, 10),
                speed_limit: parseInt(document.getElementById('rule-speed').value, 10) || 0,
            };

            if (!rule.listen_port || !rule.target_addr || !rule.target_port) {
                Toast.error('请填写完整的端口和目标地址');
                return;
            }

            try {
                await API.createRule(rule);
                closeModal();
                Toast.success('规则已创建，等待节点确认');
                const nodeId = document.getElementById('rule-node-filter')?.value || '';
                loadRules(nodeId ? parseInt(nodeId, 10) : 0);
            } catch (err) {
                Toast.error('创建失败: ' + err.message);
            }
        });
    }).catch(err => {
        Toast.error('加载节点失败: ' + err.message);
    });
}

async function showEditRuleModal(id) {
    try {
        const res = await API.getRule(id);
        const rule = res.rule;

        showModal('编辑转发规则', `
            <div class="form-group">
                <label>规则名称</label>
                <input type="text" class="form-input" id="edit-rule-name" value="${escHTML(rule.name)}">
            </div>
            <div class="form-group">
                <label>协议</label>
                <select class="form-select" id="edit-rule-protocol">
                    <option value="tcp" ${rule.protocol === 'tcp' ? 'selected' : ''}>TCP</option>
                    <option value="udp" ${rule.protocol === 'udp' ? 'selected' : ''}>UDP</option>
                    <option value="tcp+udp" ${rule.protocol === 'tcp+udp' ? 'selected' : ''}>TCP+UDP</option>
                </select>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>监听端口</label>
                    <input type="number" class="form-input" id="edit-rule-listen" value="${rule.listen_port}">
                </div>
                <div class="form-group">
                    <label>限速 (KB/s, 0=无限)</label>
                    <input type="number" class="form-input" id="edit-rule-speed" value="${rule.speed_limit}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>目标地址</label>
                    <input type="text" class="form-input" id="edit-rule-addr" value="${escHTML(rule.target_addr)}">
                </div>
                <div class="form-group">
                    <label>目标端口</label>
                    <input type="number" class="form-input" id="edit-rule-port" value="${rule.target_port}">
                </div>
            </div>
        `, async () => {
            const update = {
                name: document.getElementById('edit-rule-name').value.trim(),
                protocol: document.getElementById('edit-rule-protocol').value,
                listen_port: parseInt(document.getElementById('edit-rule-listen').value, 10),
                target_addr: document.getElementById('edit-rule-addr').value.trim(),
                target_port: parseInt(document.getElementById('edit-rule-port').value, 10),
                speed_limit: parseInt(document.getElementById('edit-rule-speed').value, 10) || 0,
            };

            try {
                await API.updateRule(id, update);
                closeModal();
                Toast.success('规则已更新，等待节点确认');
                const nodeId = document.getElementById('rule-node-filter')?.value || '';
                loadRules(nodeId ? parseInt(nodeId, 10) : 0);
            } catch (err) {
                Toast.error('更新失败: ' + err.message);
            }
        });
    } catch (err) {
        Toast.error('加载规则失败: ' + err.message);
    }
}

async function confirmDeleteRule(id, name) {
    showModal('删除规则', `
        <p style="color:var(--color-danger);">确定要删除 <strong>${name}</strong> 吗？</p>
    `, async () => {
        try {
            await API.deleteRule(id);
            closeModal();
            Toast.success('规则已删除');
            const nodeId = document.getElementById('rule-node-filter')?.value || '';
            loadRules(nodeId ? parseInt(nodeId, 10) : 0);
        } catch (err) {
            Toast.error('删除失败: ' + err.message);
        }
    }, '取消', '确认删除');
}
