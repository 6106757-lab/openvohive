'use strict';
'require view';
'require form';
'require fs';
'require ui';
'require uci';
'require dom';
'require poll';

function parseJson(text) {
	try { return JSON.parse(text || '{}'); }
	catch (e) { return { ok: false, message: text || e.message }; }
}

function notifyResult(text) {
	var r = parseJson(text);
	if (r.ok === false)
		ui.addNotification(null, E('p', {}, r.message || _('操作失败')), 'danger');
	else
		ui.addNotification(null, E('p', {}, r.message || _('操作完成')), 'info');
}

function runScript(path, args) {
	return fs.exec_direct(path, args || []).then(function(t) {
		notifyResult(t);
		window.setTimeout(function() { location.reload(); }, 800);
	}).catch(function(e) {
		ui.addNotification(null, E('p', {}, e.message || String(e)), 'danger');
	});
}

function statusBadge(active) {
	return E('span', {
		'style': 'color:%s; font-weight:700;'.format(active ? '#37a24d' : '#d9534f')
	}, active ? _('运行中') : _('已停止'));
}

function memoryText(kb) {
	var v = parseInt(kb) || 0;
	return v ? '%1024.2mB RSS'.format(v * 1024) : _('未运行');
}

function progressbar(usedKb, totalKb, percent) {
	var u = (parseInt(usedKb) || 0) * 1024;
	var t = (parseInt(totalKb) || 0) * 1024;
	var p = Math.max(0, Math.min(100, parseInt(percent) || 0));
	var txt = t ? '%1024.2mB / %1024.2mB (%d%%)'.format(u, t, p) : _('未知');
	return E('div', { 'class': 'cbi-progressbar', 'title': txt },
		E('div', { 'style': 'width:%.2f%%'.format(p) }));
}

function formatBytes(bytes) {
	var v = parseInt(bytes) || 0;
	if (v >= 1048576) return '%1024.2mB'.format(v);
	return '%d KiB'.format(Math.max(0, Math.round(v / 1024)));
}

function formatSpeed(bytes) {
	var v = parseInt(bytes) || 0;
	if (v <= 0) return '0 KiB/s';
	if (v >= 1048576) return '%1024.2mB/s'.format(v);
	return '%d KiB/s'.format(Math.max(1, Math.round(v / 1024)));
}

function loadingText(text) {
	return E('em', { 'class': 'spinning' }, text || _('正在加载...'));
}

return view.extend({
	logRefreshTimer: null,
	currentLogs: '',
	statusNode: null,
	taskTimer: null,
	taskModalBody: null,
	activeTaskId: null,
	taskCompletedHandled: false,
	corePane: null,

	load: function() {
		return Promise.all([
			uci.load('openvohive'),
			fs.exec_direct('/usr/share/openvohive/status.sh', []).catch(function() { return '{}'; }),
			fs.exec_direct('/usr/share/openvohive/logs.sh', [ '100' ]).catch(function() { return ''; })
		]);
	},

	// ─── 任务系统 ───

	startCoreTask: function(type, args) {
		return fs.exec_direct('/usr/share/openvohive/task_start.sh', [ type ].concat(args || []))
			.then(function(text) {
				var result = parseJson(text);
				if (result.ok === false || !result.id) {
					ui.addNotification(null, E('p', {}, result.message || _('任务启动失败')), 'danger');
					return;
				}
				this.showTaskDialog(result.id, type);
			}.bind(this))
			.catch(function(e) {
				ui.addNotification(null, E('p', {}, e.message || String(e)), 'danger');
			});
	},

	showTaskDialog: function(id, type) {
		this.activeTaskId = id;
		this.taskCompletedHandled = false;
		this.taskModalBody = E('div', {});
		var title = type == 'install_core' ? _('安装/更新 Open-VoHive 核心') : _('任务');

		ui.showModal(title, [ this.taskModalBody ]);
		this.pollTaskStatus();
		if (this.taskTimer) window.clearInterval(this.taskTimer);
		this.taskTimer = window.setInterval(this.pollTaskStatus.bind(this), 1000);
	},

	pollTaskStatus: function() {
		if (!this.activeTaskId) return Promise.resolve();
		return fs.exec_direct('/usr/share/openvohive/task_status.sh', [ this.activeTaskId ])
			.then(function(text) {
				var s = parseJson(text);
				if (s.ok === false) {
					ui.addNotification(null, E('p', {}, s.message || _('任务状态读取失败')), 'danger');
					return;
				}
				this.updateTaskDialog(s);
				if (s.state == 'completed' || s.state == 'failed' || s.state == 'canceled')
					this.finishTaskPolling(s);
			}.bind(this))
			.catch(function(e) {
				this.updateTaskDialog({ state: 'failed', message: e.message || String(e), log: [] });
				this.finishTaskPolling({ state: 'failed' });
			}.bind(this));
	},

	cancelTask: function() {
		if (!this.activeTaskId) return Promise.resolve();
		return fs.exec_direct('/usr/share/openvohive/task_cancel.sh', [ this.activeTaskId ])
			.catch(function(e) {
				ui.addNotification(null, E('p', {}, e.message || String(e)), 'danger');
			});
	},

	finishTaskPolling: function(s) {
		if (this.taskTimer) { window.clearInterval(this.taskTimer); this.taskTimer = null; }
		if (this.taskCompletedHandled) return;
		this.taskCompletedHandled = true;
		if (s.state == 'completed') {
			return this.refreshStatus().then(function() {
				if (this.corePane) {
					this.corePane.removeAttribute('data-loaded');
					this.corePane.removeAttribute('data-loading');
					return this.loadCorePane(this.corePane, true);
				}
			}.bind(this));
		}
	},

	updateTaskDialog: function(s) {
		if (!this.taskModalBody) return;
		var state = s.state || 'running';
		var msg = s.message || _('正在执行任务');
		var terminal = state == 'completed' || state == 'failed' || state == 'canceled';
		var total = parseInt(s.total) || 0;
		var downloaded = parseInt(s.downloaded) || 0;
		var percent = state == 'completed' ? 100 : Math.max(0, Math.min(100, parseInt(s.percent) || 0));
		var hasStats = total > 0 || downloaded > 0;
		var stats = terminal && !hasStats ? '' : (total > 0
			? '%s / %s · %s · %d%%'.format(formatBytes(downloaded), formatBytes(total), formatSpeed(s.speed_bps), percent)
			: '%s · %s'.format(formatBytes(downloaded), formatSpeed(s.speed_bps)));
		var logLines = s.log || [];
		var success = state == 'completed';

		dom.content(this.taskModalBody, E('div', { 'style': 'min-width:min(560px, 86vw);' }, [
			E('div', { 'class': 'alert-message %s'.format(success ? 'success' : (state == 'failed' || state == 'canceled' ? 'warning' : 'info')) }, msg),
			E('div', { 'class': 'cbi-progressbar', 'style': 'margin:.75em 0;', 'title': '%d%%'.format(percent) },
				E('div', { 'style': 'width:%.2f%%'.format(percent) })),
			E('div', { 'style': 'display:flex; gap:1em; flex-wrap:wrap; margin-bottom:1em;' }, [
				E('strong', {}, s.stage || state),
				E('span', {}, s.file || ''),
				E('span', {}, stats)
			]),
			E('pre', { 'style': 'white-space:pre-wrap;max-height:240px;overflow:auto;margin:0 0 1em 0;padding:1em;border:1px solid var(--border-color-medium);border-radius:6px;background:var(--background-color-low);font-family:ui-monospace,monospace;font-size:12px;' },
				logLines.length ? logLines.join('\n') : _('暂无日志')),
			E('div', { 'class': 'right' }, [
				!terminal ? E('button', { 'class': 'btn cbi-button cbi-button-reset', 'click': ui.createHandlerFn(this, this.cancelTask) }, _('取消下载')) : '',
				' ',
				E('button', { 'class': 'btn cbi-button cbi-button-neutral',
					'click': ui.createHandlerFn(this, function() {
						ui.hideModal();
						if (terminal && success) location.reload();
					})
				}, terminal ? _('完成') : _('关闭'))
			])
		]));
	},

	// ─── 核心管理 ───

	renderCorePane: function(status, releases) {
		var current = status.core_installed ? (status.core_version || _('已安装')) : _('未安装');
		var latestVer = releases.loading ? loadingText(_('正在加载...')) : (releases.latest || _('未知'));
		var canUpdate = status.core_installed && releases.latest && status.core_version && status.core_version != releases.latest;
		var isLatest = status.core_installed && releases.latest && status.core_version == releases.latest;
		var arch = status.core_arch || releases.detected_arch || _('未知');
		var archLabel = arch == 'amd64' ? 'x86_64' : (arch == 'arm64' ? 'ARM64' : (arch == 'armv7' ? 'ARMv7' : arch));

		if (canUpdate) {
			latestVer = E('span', {}, [latestVer, ' ', E('span', { 'style': 'color:#d58512;' }, _('(可更新)')) ]);
		} else if (isLatest) {
			latestVer = E('span', {}, [latestVer, ' ', E('span', { 'style': 'color:#37a24d;' }, _('(已是最新版本)')) ]);
		}

		var rows = [
			[ _('当前版本'), current ],
			[ _('设备架构'), archLabel ],
			[ _('最新版本'), latestVer ],
			[ _('Release 仓库'), E('a', { 'href': 'https://github.com/6106757-lab/openvohive/releases', 'target': '_blank', 'rel': 'noreferrer' }, 'github.com/6106757-lab/openvohive') ]
		];

		var table = E('table', { 'class': 'table' }, rows.map(function(row) {
			return E('tr', {}, [ E('td', {}, row[0]), E('td', {}, row[1]) ]);
		}));

		var nodes = [
			E('div', { 'class': 'cbi-section' }, [
				E('div', { 'style': 'display:flex; align-items:center; justify-content:space-between; gap:1em; flex-wrap:wrap;' }, [
					E('h3', { 'style': 'margin-bottom:.75em;' }, _('核心状态')),
					E('button', { 'class': 'btn cbi-button cbi-button-reload',
						'click': ui.createHandlerFn(this, function() {
							if (this.corePane) { this.corePane.removeAttribute('data-loaded'); return this.loadCorePane(this.corePane, true); }
						})
					}, _('检测更新'))
				]),
				table
			])
		];

		if (releases.ok === false)
			nodes.unshift(E('div', { 'class': 'alert-message warning' }, releases.message || _('无法获取 Release 版本列表。')));

		// 版本选择 + 安装/更新按钮（无论是否已安装都显示）
		var versionOpts = [];
		if (!releases.loading && releases.versions) {
			(releases.versions || []).forEach(function(v) {
				versionOpts.push(E('option', { 'value': v }, v));
			});
		}

		nodes.push(E('div', { 'class': 'cbi-section' }, [
			E('h3', {}, status.core_installed ? _('更新核心') : _('安装核心')),
			E('p', {}, status.core_installed
				? _('选择版本后点击安装/更新，将自动下载对应架构的核心二进制并替换。')
				: _('选择版本后点击安装，将自动检测设备架构并从 GitHub Release 下载核心。')),
			E('div', { 'style': 'display:flex; gap:.5em; flex-wrap:wrap; align-items:center;' }, [
				E('select', {
					'id': 'core-version-select',
					'style': 'padding:.4em .6em; border-radius:6px; border:1px solid var(--border-color-medium);'
				}, [
					E('option', { 'value': 'latest' }, releases.latest ? _('最新版本 (%s)').format(releases.latest) : _('最新版本'))
				].concat(versionOpts)),
				E('button', {
					'class': 'btn cbi-button cbi-button-apply',
					'click': ui.createHandlerFn(this, function() {
						var sel = document.getElementById('core-version-select');
						var ver = sel ? sel.value : 'latest';
						return this.startCoreTask('install_core', [ ver ]);
					})
				}, status.core_installed ? _('安装/更新') : _('安装')),
				canUpdate ? E('span', { 'style': 'color:#d58512; font-weight:600;' }, _('新版本可用!')) : ''
			])
		]));

		return E('div', {}, nodes);
	},

	loadCorePane: function(corePane, force) {
		if (!force && (corePane.getAttribute('data-loaded') === 'true' || corePane.getAttribute('data-loading') === 'true'))
			return;

		corePane.setAttribute('data-loading', 'true');

		return fs.exec_direct('/usr/share/openvohive/status.sh', []).catch(function() { return '{}'; })
			.then(function(text) {
				var status = parseJson(text);
				dom.content(corePane, this.renderCorePane(status, { loading: true, versions: [] }));

				return fs.exec_direct('/usr/share/openvohive/releases.sh', [ '5' ])
					.catch(function(e) {
						return JSON.stringify({ ok: false, message: e.message || String(e), latest: '', versions: [] });
					})
					.then(function(rText) {
						var releases = parseJson(rText);
						corePane.setAttribute('data-loaded', 'true');
						corePane.removeAttribute('data-loading');
						dom.content(corePane, this.renderCorePane(status, releases));
					}.bind(this));
			}.bind(this));
	},

	// ─── 运行状态 ───

	renderStatus: function(status) {
		var serverPort = status.server_port || '7575';
		var webUrl = 'http://%s:%s'.format(window.location.hostname, serverPort);
		var portLabel = status.port_status == 'listening' ? _('监听中 (:%s)').format(serverPort) : (status.port_status == 'free' ? _('端口空闲') : _('未知'));
		var rows = [
			[ _('服务状态'), statusBadge(status.running) ],
			[ _('开机启用'), status.enabled == '1' ? _('已启用') : _('未启用') ],
			[ _('核心版本'), status.core_installed ? (status.core_version || _('已安装')) : _('未安装') ],
			[ _('设备架构'), status.core_arch || _('未知') ],
			[ _('Web 管理'), status.running ? E('a', { 'href': webUrl, 'target': '_blank' }, 'Open-VoHive Web UI') : _('未运行') ],
			[ _('端口状态'), portLabel ],
			[ _('内存占用'), status.running ? memoryText(status.memory_used_kb) : _('未运行') ],
			[ _('根分区空间'), progressbar(status.root_used_kb, status.root_total_kb, status.root_percent) ],
			[ _('数据目录空间'), progressbar(status.data_used_kb, status.data_total_kb, status.data_percent) ]
		];

		var table = E('table', { 'class': 'table' }, rows.map(function(row) {
			return E('tr', {}, [ E('td', {}, row[0]), E('td', {}, row[1]) ]);
		}));

		var warnings = [];
		if (!status.core_installed)
			warnings.push(E('div', { 'class': 'alert-message warning' }, _('Open-VoHive 核心尚未安装，请在"核心管理"标签页中安装。')));
		if (status.core_installed && !status.running && status.enabled == '0')
			warnings.push(E('div', { 'class': 'alert-message warning' }, _('服务未启用，请点击下方"启用并启动"按钮。')));

		return E('div', { 'class': 'cbi-section' }, [
			E('div', { 'style': 'display:flex; align-items:center; justify-content:space-between; gap:1em; flex-wrap:wrap;' }, [
				E('h3', { 'style': 'margin-bottom:.75em;' }, _('运行状态')),
				status.running ? E('a', { 'class': 'btn cbi-button cbi-button-action', 'target': '_blank', 'href': webUrl }, _('打开 Open-VoHive Web UI')) : ''
			]),
			table
		].concat(warnings));
	},

	updateStatusNode: function(status) {
		if (!this.statusNode) return;
		dom.content(this.statusNode, this.renderStatus(status));
	},

	refreshStatus: function() {
		return fs.exec_direct('/usr/share/openvohive/status.sh', [])
			.catch(function() { return '{}'; })
			.then(function(text) {
				var status = parseJson(text);
				this.updateStatusNode(status);
				return status;
			}.bind(this));
	},

	// ─── 服务操作 ───

	renderServiceButtons: function() {
		return E('div', { 'class': 'cbi-section' }, [
			E('h3', {}, _('服务操作')),
			E('button', { 'class': 'btn cbi-button cbi-button-apply',
				'click': ui.createHandlerFn(this, function() { return runScript('/usr/share/openvohive/service.sh', [ 'start' ]); })
			}, _('启用并启动')),
			' ',
			E('button', { 'class': 'btn cbi-button cbi-button-reset',
				'click': ui.createHandlerFn(this, function() { return runScript('/usr/share/openvohive/service.sh', [ 'stop' ]); })
			}, _('停止并禁用')),
			' ',
			E('button', { 'class': 'btn cbi-button cbi-button-reload',
				'click': ui.createHandlerFn(this, function() { return runScript('/usr/share/openvohive/service.sh', [ 'restart' ]); })
			}, _('重启'))
		]);
	},

	// ─── 基础配置 ───

	renderConfigMap: function() {
		var m = new form.Map('openvohive');
		var s, o;

		s = m.section(form.NamedSection, 'main', 'openvohive', _('基础配置'));
		s.addremove = false;

		o = s.option(form.Flag, 'enabled', _('开机自启'));
		o.default = '0';
		o.rmempty = false;

		o = s.option(form.Value, 'bin_path', _('二进制路径'));
		o.default = '/opt/openvohive/openvohive';
		o.validate = function(section_id, value) {
			return /^\/.+/.test(value) || _('必须是绝对路径');
		};

		o = s.option(form.Value, 'config_path', _('配置文件路径'));
		o.default = '/opt/openvohive/config/config.yaml';

		o = s.option(form.Value, 'data_path', _('数据目录'));
		o.default = '/opt/openvohive/data';

		o = s.option(form.Value, 'log_path', _('日志目录'));
		o.default = '/opt/openvohive/logs';

		o = s.option(form.Button, '_apply_config', _('保存并应用'));
		o.inputstyle = 'apply';
		o.onclick = ui.createHandlerFn(this, function() {
			return m.save().then(function() {
				return ui.changes.apply(false).then(function() {
					ui.addNotification(null, E('p', {}, _('配置已保存。如修改了二进制路径，请手动重启服务。')), 'info');
				});
			});
		});

		return m.render();
	},

	// ─── 日志 ───

	refreshLogs: function(logNode) {
		return fs.exec_direct('/usr/share/openvohive/logs.sh', [ '100' ])
			.catch(function() { return ''; })
			.then(function(logs) {
				this.currentLogs = logs || '';
				dom.content(logNode, this.currentLogs || _('暂无日志'));
			}.bind(this));
	},

	setLogAutoRefresh: function(enabled, logNode) {
		if (this.logRefreshTimer) { window.clearInterval(this.logRefreshTimer); this.logRefreshTimer = null; }
		if (enabled) {
			this.refreshLogs(logNode);
			this.logRefreshTimer = window.setInterval(function() { this.refreshLogs(logNode); }.bind(this), 5000);
		}
	},

	clearLogs: function(logNode) {
		return fs.exec_direct('/usr/share/openvohive/clear_logs.sh', [])
			.then(function(text) { notifyResult(text); return this.refreshLogs(logNode); }.bind(this))
			.catch(function(e) { ui.addNotification(null, E('p', {}, e.message || String(e)), 'danger'); });
	},

	downloadLogs: function() {
		var blob = new Blob([ this.currentLogs || '' ], { type: 'text/plain;charset=utf-8' });
		var url = URL.createObjectURL(blob);
		var a = E('a', { 'href': url, 'download': 'openvohive-logs.txt' });
		document.body.appendChild(a); a.click(); document.body.removeChild(a);
		window.setTimeout(function() { URL.revokeObjectURL(url); }, 1000);
	},

	renderLogs: function(logs) {
		this.currentLogs = logs || '';
		var logNode = E('pre', {
			'style': 'white-space:pre;height:460px;overflow:auto;margin:0;padding:1em;border:1px solid var(--border-color-medium);border-radius:6px;background:var(--background-color-low);font-family:ui-monospace,monospace;font-size:12px;line-height:1.55;'
		}, this.currentLogs || _('暂无日志'));

		return E('div', { 'class': 'cbi-section' }, [
			E('h3', {}, _('运行日志')),
			E('div', { 'style': 'display:flex;align-items:center;justify-content:space-between;gap:1em;flex-wrap:wrap;margin-bottom:1em;' }, [
				E('label', { 'style': 'display:inline-flex;align-items:center;gap:.5em;margin:0;' }, [
					E('input', { 'type': 'checkbox', 'style': 'margin:0;',
						'change': function(ev) { this.setLogAutoRefresh(ev.target.checked, logNode); }.bind(this)
					}),
					E('span', {}, _('自动刷新'))
				]),
				E('div', { 'style': 'display:flex;gap:.5em;flex-wrap:wrap;' }, [
					E('button', { 'class': 'btn cbi-button cbi-button-reload',
						'click': ui.createHandlerFn(this, function() { return this.refreshLogs(logNode); })
					}, _('刷新')),
					E('button', { 'class': 'btn cbi-button cbi-button-reset',
						'click': ui.createHandlerFn(this, function() { return this.clearLogs(logNode); })
					}, _('清理日志')),
					E('button', { 'class': 'btn cbi-button cbi-button-action',
						'click': ui.createHandlerFn(this, function() { this.downloadLogs(); })
					}, _('下载日志'))
				])
			]),
			logNode
		]);
	},

	// ─── 主渲染 ───

	render: function(data) {
		var status = parseJson(data[1]);
		var logs = data[2] || '';

		this.statusNode = E('div', {}, this.renderStatus(status));
		poll.add(this.refreshStatus.bind(this), 5);

		var corePane = E('div', { 'data-tab': 'core', 'data-tab-title': _('核心管理') }, [
			E('div', { 'class': 'cbi-section' }, E('em', {}, _('点击核心管理后加载版本列表。')))
		]);
		this.corePane = corePane;
		corePane.addEventListener('cbi-tab-active', function() {
			this.loadCorePane(corePane);
		}.bind(this));

		// renderConfigMap 返回 Promise，需要等待
		return Promise.resolve(this.renderConfigMap()).then(function(configEl) {
			var panes = E('div', {}, [
				E('div', { 'data-tab': 'home', 'data-tab-title': _('首页') }, [
					this.statusNode,
					this.renderServiceButtons()
				]),
				corePane,
				E('div', { 'data-tab': 'config', 'data-tab-title': _('基础配置') }, configEl),
				E('div', { 'data-tab': 'logs', 'data-tab-title': _('日志') }, this.renderLogs(logs))
			]);

			var root = E('div', {}, [
				E('h2', {}, _('Open-VoHive')),
				E('div', { 'class': 'cbi-map-descr' }, _('管理 Open-VoHive 4G/5G 模组管理器。支持服务控制、核心更新、状态监控与日志查看。')),
				panes
			]);

			// 延迟初始化 tabs，确保 DOM 已挂载
			window.setTimeout(function() {
				ui.tabs.initTabGroup(panes.childNodes);
			}, 0);

			return root;
		}.bind(this));
	}
});
