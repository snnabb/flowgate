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
                    <p class="subtitle">管理端口转发规则</p>
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
    loadRules(nodeFilter ? parseInt(nodeFilter) : 0);
}

async function loadNodeOptions(selectedId) {
    try {
        const res = await API.getNodes();
        const select = document.getElementById('rule-node-filter');
        (res.nodes || []).forEach(n => {
            const opt = document.createElement('option');
            opt.value = n.id;
            opt.textContent = n.name;
            if (String(n.id) === selectedId) opt.selected = true;
            select.appendChild(opt);
        });
    } catch(e) {}
}

function filterRulesByNode() {
    const nodeId = document.getElementById('rule-node-filter').value;
    const url = nodeId ? `/rules?node_id=${nodeId}` : '/rules';
    window.history.replaceState({}, '', url);
    loadRules(nodeId ? parseInt(nodeId) : 0);
}

// Cache nodes for name lookup
let _nodesCache = {};

async function loadRules(nodeId) {
    try {
        const [rulesRes, nodesRes] = await Promise.all([
            API.getRules(nodeId || ''),
            API.getNodes()
        ]);

        _nodesCache = {};
        (nodesRes.nodes || []).forEach(n => { _nodesCache[n.id] = n.name; });

        const rules = rulesRes.rules || [];
        const body = document.getElementById('rules-body');

        if (rules.length === 0) {
            body.innerHTML = '<tr><td colspan="11" class="empty-state"><p>暂无转发规则</p></td></tr>';
            return;
        }

        body.innerHTML = rules.map(r => {
            const protoClass = r.protocol === 'tcp' ? 'tcp' : r.protocol === 'udp' ? 'udp' : 'both';
            const speedText = r.speed_limit > 0 ? `${r.speed_limit} KB/s` : '无限';
            return `
                <tr>
                    <td>#${r.id}</td>
                    <td>${escHTML(r.name || `规则 #${r.id}`)}</td>
                    <td>${escHTML(_nodesCache[r.node_id] || '#' + r.node_id)}</td>
                    <td><span class="badge badge-${protoClass}">${r.protocol.toUpperCase()}</span></td>
                    <td><strong>${r.listen_port}</strong></td>
                    <td>${escHTML(r.target_addr)}:${r.target_port}</td>
                    <td>${speedText}</td>
                    <td>${formatBytes(r.traffic_in)}</td>
                    <td>${formatBytes(r.traffic_out)}</td>
                    <td>
                        <label class="toggle" title="${r.enabled ? '启用' : '禁用'}">
                            <input type="checkbox" ${r.enabled ? 'checked' : ''} onchange="toggleRuleEnabled(${r.id})">
                            <span class="toggle-slider"></span>
                        </label>
                    </td>
                    <td>
                        <div class="action-group">
                            <button class="btn btn-sm btn-secondary" onclick="showEditRuleModal(${r.id})" title="编辑">✏️</button>
                            <button class="btn btn-sm btn-danger" onclick="confirmDeleteRule(${r.id}, '${escHTML(r.name || '规则 #' + r.id)}')" title="删除">🗑</button>
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
    } catch (err) {
        Toast.error('操作失败: ' + err.message);
        // Reload to revert toggle state
        const nodeId = document.getElementById('rule-node-filter')?.value || '';
        loadRules(nodeId ? parseInt(nodeId) : 0);
    }
}

function showCreateRuleModal() {
    // Load nodes for select
    API.getNodes().then(res => {
        const nodes = res.nodes || [];
        const nodeOpts = nodes.map(n => `<option value="${n.id}">${escHTML(n.name)}</option>`).join('');

        showModal('添加转发规则', `
            <div class="form-group">
                <label>规则名称</label>
                <input type="text" class="form-input" id="rule-name" placeholder="例如: 游戏加速">
            </div>
            <div class="form-group">
                <label>节点</label>
                <select class="form-select" id="rule-node">${nodeOpts}</select>
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
                node_id: parseInt(document.getElementById('rule-node').value),
                protocol: document.getElementById('rule-protocol').value,
                listen_port: parseInt(document.getElementById('rule-listen-port').value),
                target_addr: document.getElementById('rule-target-addr').value.trim(),
                target_port: parseInt(document.getElementById('rule-target-port').value),
                speed_limit: parseInt(document.getElementById('rule-speed').value) || 0,
            };

            if (!rule.listen_port || !rule.target_addr || !rule.target_port) {
                Toast.error('请填写完整的端口和目标地址');
                return;
            }

            try {
                await API.createRule(rule);
                closeModal();
                Toast.success('规则创建成功，已推送到节点');
                const nodeId = document.getElementById('rule-node-filter')?.value || '';
                loadRules(nodeId ? parseInt(nodeId) : 0);
            } catch (err) {
                Toast.error('创建失败: ' + err.message);
            }
        });
    });
}

async function showEditRuleModal(id) {
    try {
        const res = await API.getRule(id);
        const r = res.rule;

        showModal('编辑转发规则', `
            <div class="form-group">
                <label>规则名称</label>
                <input type="text" class="form-input" id="edit-rule-name" value="${escHTML(r.name)}">
            </div>
            <div class="form-group">
                <label>协议</label>
                <select class="form-select" id="edit-rule-protocol">
                    <option value="tcp" ${r.protocol==='tcp'?'selected':''}>TCP</option>
                    <option value="udp" ${r.protocol==='udp'?'selected':''}>UDP</option>
                    <option value="tcp+udp" ${r.protocol==='tcp+udp'?'selected':''}>TCP+UDP</option>
                </select>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>监听端口</label>
                    <input type="number" class="form-input" id="edit-rule-listen" value="${r.listen_port}">
                </div>
                <div class="form-group">
                    <label>限速 (KB/s, 0=无限)</label>
                    <input type="number" class="form-input" id="edit-rule-speed" value="${r.speed_limit}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>目标地址</label>
                    <input type="text" class="form-input" id="edit-rule-addr" value="${escHTML(r.target_addr)}">
                </div>
                <div class="form-group">
                    <label>目标端口</label>
                    <input type="number" class="form-input" id="edit-rule-port" value="${r.target_port}">
                </div>
            </div>
        `, async () => {
            const update = {
                name: document.getElementById('edit-rule-name').value.trim(),
                protocol: document.getElementById('edit-rule-protocol').value,
                listen_port: parseInt(document.getElementById('edit-rule-listen').value),
                target_addr: document.getElementById('edit-rule-addr').value.trim(),
                target_port: parseInt(document.getElementById('edit-rule-port').value),
                speed_limit: parseInt(document.getElementById('edit-rule-speed').value) || 0,
            };

            try {
                await API.updateRule(id, update);
                closeModal();
                Toast.success('规则已更新并推送到节点');
                const nodeId = document.getElementById('rule-node-filter')?.value || '';
                loadRules(nodeId ? parseInt(nodeId) : 0);
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
            loadRules(nodeId ? parseInt(nodeId) : 0);
        } catch (err) {
            Toast.error('删除失败: ' + err.message);
        }
    }, '确认', '确认删除');
}
