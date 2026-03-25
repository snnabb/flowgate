// Rules Component
let rulesRefreshTimer = null;

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
                <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
                    <input type="text" class="form-input" id="rule-search" placeholder="搜索规则..." style="width:140px;padding:6px 10px;font-size:0.82rem;" oninput="filterRulesBySearch()">
                    <select class="form-select" id="rule-node-filter" style="width:140px;flex-shrink:0;" onchange="filterRulesByNode()">
                        <option value="">全部节点</option>
                    </select>
                    <button class="btn btn-primary" onclick="showCreateRuleModal()">+ 添加</button>
                </div>
            </div>

            <div class="table-container desktop-only">
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
                            <th>流量</th>
                            <th>延迟</th>
                            <th>状态</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="rules-body">
                        <tr><td colspan="11" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only m-card-list" id="rules-cards">
                <p style="color:var(--text-muted);text-align:center;padding:20px;">加载中...</p>
            </div>
        </div>
    `;

    loadNodeOptions(nodeFilter);
    loadRules(nodeFilter ? parseInt(nodeFilter, 10) : 0);
    startRulesAutoRefresh();
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
let _ruleNodeGroupsCache = [];

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

async function loadRules(nodeId, silent) {
    try {
        const [rulesRes, nodesRes] = await Promise.all([
            API.getRules(nodeId || ''),
            API.getNodes()
        ]);

        _nodesCache = {};
        (nodesRes.nodes || []).forEach(node => {
            _nodesCache[node.id] = node.name;
        });

        // Store rules globally for search filtering
        window._allRules = rulesRes.rules || [];
        renderFilteredRules(window._allRules);
    } catch (err) {
        if (!silent) {
            Toast.error('加载规则失败: ' + err.message);
        }
    }
}

function filterRulesBySearch() {
    const q = (document.getElementById('rule-search')?.value || '').toLowerCase();
    if (!window._allRules) return;
    if (!q) {
        renderFilteredRules(window._allRules);
        return;
    }
    const filtered = window._allRules.filter(r => {
        const name = (r.name || '').toLowerCase();
        const target = (r.target_addr + ':' + r.target_port).toLowerCase();
        const node = (_nodesCache[r.node_id] || '').toLowerCase();
        return name.includes(q) || target.includes(q) || node.includes(q) || String(r.listen_port).includes(q);
    });
    renderFilteredRules(filtered);
}

function renderFilteredRules(rules) {
    const body = document.getElementById('rules-body');
    const cards = document.getElementById('rules-cards');

    if (rules.length === 0) {
        if (body) body.innerHTML = '<tr><td colspan="11" class="empty-state"><p>暂无转发规则</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">暂无转发规则</p>';
        return;
    }

    if (body) body.innerHTML = rules.map(rule => {
        const protoClass = rule.protocol === 'tcp' ? 'tcp' : rule.protocol === 'udp' ? 'udp' : 'both';
        const speedText = rule.speed_limit > 0 ? `${rule.speed_limit} KB/s` : '无限';
        const totalTraffic = rule.traffic_in + rule.traffic_out;
        const trafficCell = rule.traffic_limit > 0
            ? formatTrafficWithLimit(totalTraffic, rule.traffic_limit)
            : `${formatBytes(rule.traffic_in)} / ${formatBytes(rule.traffic_out)}`;
        return `
            <tr>
                <td>#${rule.id}</td>
                <td>${escHTML(rule.name || `规则 #${rule.id}`)}</td>
                <td>${escHTML(_nodesCache[rule.node_id] || `#${rule.node_id}`)}</td>
                <td><span class="badge badge-${protoClass}">${rule.protocol.toUpperCase()}</span>${renderTunnelBadges(rule)}${renderRouteBadges(rule)}</td>
                <td><strong>${rule.listen_port}</strong></td>
                <td>${escHTML(rule.target_addr)}:${rule.target_port}</td>
                <td>${speedText}</td>
                <td>${trafficCell}</td>
                <td>${formatLatency(rule.latency_ms)}</td>
                <td>${renderRuleStatus(rule)}</td>
                <td>
                    <div class="action-group">
                        <button class="btn btn-sm btn-secondary" onclick="showEditRuleModal(${rule.id})" title="编辑">✎</button>
                        <button class="btn btn-sm btn-secondary" onclick="confirmResetTraffic(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')" title="重置流量">↺</button>
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteRule(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')" title="删除">🗑</button>
                    </div>
                </td>
            </tr>
        `;
    }).join('');

    if (cards) cards.innerHTML = rules.map(rule => {
        const protoClass = rule.protocol === 'tcp' ? 'tcp' : rule.protocol === 'udp' ? 'udp' : 'both';
        const runtime = getRuleRuntimeMeta(rule);
        const totalTraffic = rule.traffic_in + rule.traffic_out;
        const trafficDisplay = rule.traffic_limit > 0
            ? formatTrafficWithLimit(totalTraffic, rule.traffic_limit)
            : `<span style="color:var(--color-info);">↓${formatBytes(rule.traffic_in)}</span> <span style="color:var(--color-success);">↑${formatBytes(rule.traffic_out)}</span>`;
        return `
            <div class="m-card">
                <div class="m-card-head">
                    <span class="m-card-title">${escHTML(rule.name || `规则 #${rule.id}`)}</span>
                    <span class="m-card-id">#${rule.id}</span>
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">节点</span>
                        <span class="m-card-val">${escHTML(_nodesCache[rule.node_id] || `#${rule.node_id}`)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">协议 / 端口</span>
                        <span class="m-card-val"><span class="badge badge-${protoClass}" style="font-size:0.7rem;padding:2px 8px;">${rule.protocol.toUpperCase()}</span>${renderTunnelBadges(rule)}${renderRouteBadges(rule)} :${rule.listen_port}</span>
                    </div>
                    <div class="m-card-row full">
                        <span class="m-card-label">目标</span>
                        <span class="m-card-val">${escHTML(rule.target_addr)}:${rule.target_port}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">流量</span>
                        <span class="m-card-val">${trafficDisplay}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">延迟</span>
                        <span class="m-card-val">${formatLatency(rule.latency_ms)}</span>
                    </div>
                </div>
                <div class="m-card-foot">
                    <div class="m-card-status">
                        <span class="badge badge-${runtime.badge}"><span class="badge-dot"></span>${runtime.label}</span>
                        <label class="toggle" title="${rule.enabled ? '启用' : '禁用'}">
                            <input type="checkbox" ${rule.enabled ? 'checked' : ''} onchange="toggleRuleEnabled(${rule.id})">
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="action-group" style="display:flex;gap:6px;">
                        <button class="btn btn-sm btn-secondary" onclick="showEditRuleModal(${rule.id})">编辑</button>
                        <button class="btn btn-sm btn-secondary" onclick="confirmResetTraffic(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')">重置</button>
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteRule(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')">删除</button>
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

function startRulesAutoRefresh() {
    if (rulesRefreshTimer) {
        clearInterval(rulesRefreshTimer);
    }

    rulesRefreshTimer = setInterval(() => {
        if (Router.currentPath !== '/rules') {
            clearInterval(rulesRefreshTimer);
            rulesRefreshTimer = null;
            return;
        }

        const nodeId = document.getElementById('rule-node-filter')?.value || '';
        loadRules(nodeId ? parseInt(nodeId, 10) : 0, true);
    }, 5000);
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
    Promise.all([API.getNodes(), API.getNodeGroups()]).then(([res, groupsRes]) => {
        const nodes = res.nodes || [];
        _ruleNodeGroupsCache = groupsRes.node_groups || [];
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
            <div class="form-group">
                <label>流量限额 (0=无限, 例如: 100GB, 500MB)</label>
                <input type="text" class="form-input" id="rule-traffic-limit" placeholder="例如: 100GB 或 0" value="0">
            </div>
            ${renderRouteSettings('rule')}
            ${renderTunnelSettings('rule')}
        `, async () => {
            if (!validateRouteFormSettings('rule') || !validateTunnelFormSettings('rule')) {
                return;
            }

            const rule = {
                name: document.getElementById('rule-name').value.trim(),
                node_id: parseInt(document.getElementById('rule-node').value, 10),
                protocol: document.getElementById('rule-protocol').value,
                listen_port: parseInt(document.getElementById('rule-listen-port').value, 10),
                target_addr: document.getElementById('rule-target-addr').value.trim(),
                target_port: parseInt(document.getElementById('rule-target-port').value, 10),
                speed_limit: parseInt(document.getElementById('rule-speed').value, 10) || 0,
                traffic_limit: parseTrafficLimit(document.getElementById('rule-traffic-limit').value),
                ...parseRouteSettings('rule'),
                ...parseTunnelSettings('rule'),
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
        syncRouteMode('rule');
        syncTunnelCompatibility('rule');
    }).catch(err => {
        Toast.error('加载节点失败: ' + err.message);
    });
}

async function showEditRuleModal(id) {
    try {
        const [res, groupsRes] = await Promise.all([API.getRule(id), API.getNodeGroups()]);
        _ruleNodeGroupsCache = groupsRes.node_groups || [];
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
            <div class="form-group">
                <label>流量限额 (0=无限, 例如: 100GB, 500MB)</label>
                <input type="text" class="form-input" id="edit-rule-traffic-limit" value="${rule.traffic_limit > 0 ? formatTrafficLimitInput(rule.traffic_limit) : '0'}">
            </div>
            ${renderRouteSettings('edit-rule', rule)}
            ${renderTunnelSettings('edit-rule', rule)}
        `, async () => {
            if (!validateRouteFormSettings('edit-rule') || !validateTunnelFormSettings('edit-rule')) {
                return;
            }

            const update = {
                name: document.getElementById('edit-rule-name').value.trim(),
                protocol: document.getElementById('edit-rule-protocol').value,
                listen_port: parseInt(document.getElementById('edit-rule-listen').value, 10),
                target_addr: document.getElementById('edit-rule-addr').value.trim(),
                target_port: parseInt(document.getElementById('edit-rule-port').value, 10),
                speed_limit: parseInt(document.getElementById('edit-rule-speed').value, 10) || 0,
                traffic_limit: parseTrafficLimit(document.getElementById('edit-rule-traffic-limit').value),
                ...parseRouteSettings('edit-rule'),
                ...parseTunnelSettings('edit-rule'),
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
        syncRouteMode('edit-rule');
        syncTunnelCompatibility('edit-rule');
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

async function confirmResetTraffic(id, name) {
    showModal('重置流量', `
        <p>确定要重置 <strong>${name}</strong> 的流量计数器吗？</p>
        <p style="color:var(--text-muted);font-size:0.85rem;margin-top:8px;">此操作将清零入站和出站流量统计。</p>
    `, async () => {
        try {
            await API.resetTraffic(id);
            closeModal();
            Toast.success('流量已重置');
            const nodeId = document.getElementById('rule-node-filter')?.value || '';
            loadRules(nodeId ? parseInt(nodeId, 10) : 0);
        } catch (err) {
            Toast.error('重置失败: ' + err.message);
        }
    }, '取消', '确认重置');
}

function renderNodeGroupOptionTags(selectedValue) {
    const current = selectedValue || '';
    const options = ['<option value="">请选择</option>'];
    (_ruleNodeGroupsCache || []).forEach(group => {
        options.push(`<option value="${escHTML(group.name)}" ${group.name === current ? 'selected' : ''}>${escHTML(group.name)}</option>`);
    });
    if (current && !(_ruleNodeGroupsCache || []).some(group => group.name === current)) {
        options.push(`<option value="${escHTML(current)}" selected>${escHTML(current)}</option>`);
    }
    return options.join('');
}

function renderRouteSettings(prefix, rule) {
    const routeMode = rule ? (rule.route_mode || 'direct') : 'direct';
    const entryGroup = rule ? (rule.entry_group || '') : '';
    const relayGroups = rule ? (rule.relay_groups || '') : '';
    const exitGroup = rule ? (rule.exit_group || '') : '';
    const lbStrategy = rule ? (rule.lb_strategy || 'none') : 'none';

    return `
        <div class="tunnel-section">
            <div class="tunnel-toggle" onclick="this.parentElement.classList.toggle('open')">
                <span>链路设置</span>
                <span class="tunnel-arrow">▾</span>
            </div>
            <div class="tunnel-body">
                <div class="form-row">
                    <div class="form-group">
                        <label>路由模式</label>
                        <select class="form-select" id="${prefix}-route-mode" onchange="syncRouteMode('${prefix}')">
                            <option value="direct" ${routeMode === 'direct' ? 'selected' : ''}>直连</option>
                            <option value="group_chain" ${routeMode === 'group_chain' ? 'selected' : ''}>分组链路</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>负载策略</label>
                        <select class="form-select" id="${prefix}-lb-strategy">
                            <option value="none" ${lbStrategy === 'none' ? 'selected' : ''}>关闭</option>
                            <option value="round_robin" ${lbStrategy === 'round_robin' ? 'selected' : ''}>轮询</option>
                            <option value="weighted_round_robin" ${lbStrategy === 'weighted_round_robin' ? 'selected' : ''}>加权轮询</option>
                            <option value="least_connections" ${lbStrategy === 'least_connections' ? 'selected' : ''}>最小连接</option>
                            <option value="least_latency" ${lbStrategy === 'least_latency' ? 'selected' : ''}>最小延迟</option>
                            <option value="ip_hash" ${lbStrategy === 'ip_hash' ? 'selected' : ''}>IP Hash</option>
                            <option value="failover" ${lbStrategy === 'failover' ? 'selected' : ''}>主备故障转移</option>
                        </select>
                    </div>
                </div>
                <div id="${prefix}-route-chain-fields">
                    <div class="form-row">
                        <div class="form-group">
                            <label>入口组</label>
                            <select class="form-select" id="${prefix}-entry-group">${renderNodeGroupOptionTags(entryGroup)}</select>
                        </div>
                        <div class="form-group">
                            <label>出口组</label>
                            <select class="form-select" id="${prefix}-exit-group">${renderNodeGroupOptionTags(exitGroup)}</select>
                        </div>
                    </div>
                    <div class="form-group">
                        <label>中转组 (可选，逗号分隔)</label>
                        <input type="text" class="form-input" id="${prefix}-relay-groups" value="${escHTML(relayGroups)}" placeholder="relay-sg,relay-jp">
                    </div>
                </div>
                <div id="${prefix}-route-note" style="display:none;color:var(--color-warning, #e6a23c);font-size:0.8rem;">
                    分组链路当前仅保存配置并显示在面板中，运行时还未接入节点转发。
                </div>
            </div>
        </div>
    `;
}

function syncRouteMode(prefix) {
    const mode = document.getElementById(prefix + '-route-mode')?.value || 'direct';
    const chainFields = document.getElementById(prefix + '-route-chain-fields');
    const note = document.getElementById(prefix + '-route-note');
    const lbSelect = document.getElementById(prefix + '-lb-strategy');

    if (chainFields) {
        chainFields.style.display = mode === 'group_chain' ? 'block' : 'none';
    }
    if (note) {
        note.style.display = mode === 'group_chain' ? 'block' : 'none';
    }
    if (lbSelect) {
        lbSelect.disabled = mode !== 'group_chain';
        if (mode !== 'group_chain') {
            lbSelect.value = 'none';
        }
    }
}

function parseRouteSettings(prefix) {
    const routeMode = document.getElementById(prefix + '-route-mode')?.value || 'direct';
    if (routeMode !== 'group_chain') {
        return {
            route_mode: 'direct',
            entry_group: '',
            relay_groups: '',
            exit_group: '',
            lb_strategy: 'none',
        };
    }

    return {
        route_mode: routeMode,
        entry_group: document.getElementById(prefix + '-entry-group')?.value?.trim() || '',
        relay_groups: document.getElementById(prefix + '-relay-groups')?.value?.trim() || '',
        exit_group: document.getElementById(prefix + '-exit-group')?.value?.trim() || '',
        lb_strategy: document.getElementById(prefix + '-lb-strategy')?.value || 'none',
    };
}

function validateRouteFormSettings(prefix) {
    const settings = parseRouteSettings(prefix);
    if (settings.route_mode !== 'group_chain') {
        return true;
    }
    if (!settings.entry_group || !settings.exit_group) {
        Toast.error('分组链路至少需要入口组和出口组');
        return false;
    }
    return true;
}

function renderRouteBadges(rule) {
    if (!rule || !rule.route_mode || rule.route_mode === 'direct') {
        return '';
    }

    let badges = '<span class="tunnel-badge" style="background:rgba(245,158,11,0.16);color:#b45309;" title="分组链路规则">Chain</span>';
    if (rule.lb_strategy && rule.lb_strategy !== 'none') {
        const shortLabel = {
            round_robin: 'RR',
            weighted_round_robin: 'WRR',
            least_connections: 'LC',
            least_latency: 'LL',
            ip_hash: 'Hash',
            failover: 'Failover',
        }[rule.lb_strategy] || 'LB';
        badges += `<span class="tunnel-badge" style="background:rgba(14,165,233,0.14);color:#0369a1;" title="负载策略: ${escHTML(rule.lb_strategy)}">${shortLabel}</span>`;
    }
    return badges;
}

// --- Tunnel Settings UI Helpers ---

function renderTunnelSettings(prefix, rule) {
    const pp = rule ? rule.proxy_protocol || 0 : 0;
    const bp = rule ? (rule.blocked_protos || '') : '';
    const ps = rule ? rule.pool_size || 0 : 0;
    const tm = rule ? (rule.tls_mode || 'none') : 'none';
    const ts = rule ? (rule.tls_sni || '') : '';
    const we = rule ? (rule.ws_enabled || false) : false;
    const wp = rule ? (rule.ws_path || '/ws') : '/ws';

    const hasSocks = bp.includes('socks');
    const hasHttp = bp.includes('http');
    const hasTls = bp.includes('tls');

    return `
        <div class="tunnel-section">
            <div class="tunnel-toggle" onclick="this.parentElement.classList.toggle('open')">
                <span>隧道设置</span>
                <span class="tunnel-arrow">▸</span>
            </div>
            <div class="tunnel-body">
                <div class="form-row">
                    <div class="form-group">
                        <label>PROXY Protocol</label>
                        <select class="form-select" id="${prefix}-proxy-protocol">
                            <option value="0" ${pp===0?'selected':''}>关闭</option>
                            <option value="1" ${pp===1?'selected':''}>v1 (文本)</option>
                            <option value="2" ${pp===2?'selected':''}>v2 (二进制)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>连接池大小 (0=关闭)</label>
                        <input type="number" class="form-input" id="${prefix}-pool-size" value="${ps}" min="0" max="100">
                    </div>
                </div>
                <div class="form-group">
                    <label>协议拦截</label>
                    <div style="display:flex;gap:12px;margin-top:4px;">
                        <label style="display:flex;align-items:center;gap:4px;font-size:0.85rem;cursor:pointer;">
                            <input type="checkbox" id="${prefix}-block-socks" ${hasSocks?'checked':''}> SOCKS
                        </label>
                        <label style="display:flex;align-items:center;gap:4px;font-size:0.85rem;cursor:pointer;">
                            <input type="checkbox" id="${prefix}-block-http" ${hasHttp?'checked':''}> HTTP
                        </label>
                        <label style="display:flex;align-items:center;gap:4px;font-size:0.85rem;cursor:pointer;">
                            <input type="checkbox" id="${prefix}-block-tls" ${hasTls?'checked':''}> TLS
                        </label>
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>TLS 模式</label>
                        <select class="form-select" id="${prefix}-tls-mode" onchange="toggleTlsSni('${prefix}')">
                            <option value="none" ${tm==='none'?'selected':''}>关闭</option>
                            <option value="client" ${tm==='client'?'selected':''}>客户端 (入站TLS)</option>
                            <option value="server" ${tm==='server'?'selected':''}>服务端 (出站TLS)</option>
                            <option value="both" ${tm==='both'?'selected':''}>双向 TLS</option>
                        </select>
                    </div>
                    <div class="form-group" id="${prefix}-tls-sni-group" style="display:${(tm==='server'||tm==='both')?'block':'none'}">
                        <label>TLS SNI</label>
                        <input type="text" class="form-input" id="${prefix}-tls-sni" value="${escHTML(ts)}" placeholder="example.com">
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label style="display:flex;align-items:center;gap:6px;">
                            <input type="checkbox" id="${prefix}-ws-enabled" ${we?'checked':''} onchange="toggleWsPath('${prefix}')"> WebSocket 隧道
                        </label>
                    </div>
                    <div class="form-group" id="${prefix}-ws-path-group" style="display:${we?'block':'none'}">
                        <label>WS 路径</label>
                        <input type="text" class="form-input" id="${prefix}-ws-path" value="${escHTML(wp)}" placeholder="/ws">
                    </div>
                </div>
                <div id="${prefix}-ws-tls-note" style="display:none;color:var(--color-warning, #e6a23c);font-size:0.8rem;">
                    WebSocket 隧道暂不支持与入站 TLS 同时开启。
                </div>
            </div>
        </div>
    `;
}

function toggleTlsSni(prefix) {
    const mode = document.getElementById(prefix + '-tls-mode').value;
    const group = document.getElementById(prefix + '-tls-sni-group');
    if (group) group.style.display = (mode === 'server' || mode === 'both') ? 'block' : 'none';
    syncTunnelCompatibility(prefix);
}

function toggleWsPath(prefix) {
    const enabled = document.getElementById(prefix + '-ws-enabled').checked;
    const group = document.getElementById(prefix + '-ws-path-group');
    if (group) group.style.display = enabled ? 'block' : 'none';
    syncTunnelCompatibility(prefix);
}

function syncTunnelCompatibility(prefix) {
    const wsEnabled = document.getElementById(prefix + '-ws-enabled')?.checked || false;
    const tlsSelect = document.getElementById(prefix + '-tls-mode');
    const note = document.getElementById(prefix + '-ws-tls-note');

    if (!tlsSelect) return;

    const clientOption = Array.from(tlsSelect.options).find(opt => opt.value === 'client');
    const bothOption = Array.from(tlsSelect.options).find(opt => opt.value === 'both');
    if (clientOption) clientOption.disabled = wsEnabled;
    if (bothOption) bothOption.disabled = wsEnabled;

    if (wsEnabled && (tlsSelect.value === 'client' || tlsSelect.value === 'both')) {
        tlsSelect.value = 'none';
        const sniGroup = document.getElementById(prefix + '-tls-sni-group');
        if (sniGroup) sniGroup.style.display = 'none';
    }

    if (note) note.style.display = wsEnabled ? 'block' : 'none';
}

function validateTunnelFormSettings(prefix) {
    const settings = parseTunnelSettings(prefix);
    if (settings.ws_enabled && (settings.tls_mode === 'client' || settings.tls_mode === 'both')) {
        Toast.error('WebSocket 隧道暂不支持与入站 TLS 同时开启');
        return false;
    }
    return true;
}

function parseTunnelSettings(prefix) {
    const blocked = [];
    if (document.getElementById(prefix + '-block-socks')?.checked) blocked.push('socks');
    if (document.getElementById(prefix + '-block-http')?.checked) blocked.push('http');
    if (document.getElementById(prefix + '-block-tls')?.checked) blocked.push('tls');

    return {
        proxy_protocol: parseInt(document.getElementById(prefix + '-proxy-protocol')?.value || '0', 10),
        blocked_protos: blocked.join(','),
        pool_size: parseInt(document.getElementById(prefix + '-pool-size')?.value || '0', 10),
        tls_mode: document.getElementById(prefix + '-tls-mode')?.value || 'none',
        tls_sni: document.getElementById(prefix + '-tls-sni')?.value?.trim() || '',
        ws_enabled: document.getElementById(prefix + '-ws-enabled')?.checked || false,
        ws_path: document.getElementById(prefix + '-ws-path')?.value?.trim() || '/ws',
    };
}

function renderTunnelBadges(rule) {
    let badges = '';
    if (rule.tls_mode && rule.tls_mode !== 'none') {
        badges += '<span class="tunnel-badge tunnel-badge-tls" title="TLS: ' + escHTML(rule.tls_mode) + '">TLS</span>';
    }
    if (rule.ws_enabled) {
        badges += '<span class="tunnel-badge tunnel-badge-ws" title="WebSocket 隧道">WS</span>';
    }
    if (rule.blocked_protos) {
        badges += '<span class="tunnel-badge tunnel-badge-block" title="拦截: ' + escHTML(rule.blocked_protos) + '">Block</span>';
    }
    if (rule.proxy_protocol > 0) {
        badges += '<span class="tunnel-badge tunnel-badge-pp" title="PROXY Protocol v' + rule.proxy_protocol + '">PP</span>';
    }
    if (rule.pool_size > 0) {
        badges += '<span class="tunnel-badge tunnel-badge-pool" title="连接池: ' + rule.pool_size + '">Pool</span>';
    }
    return badges;
}
