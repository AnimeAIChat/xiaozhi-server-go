(function () {
  'use strict';

  const app = document.getElementById('app');
  const toastRoot = document.getElementById('toast-root');
  const modalRoot = document.getElementById('modal-root');
  const assetBase = window.__STATIC_WEB_BASE__ || '/';
  const apiBase = location.protocol === 'file:'
    ? (localStorage.getItem('xiaozhi_api_base') || 'http://localhost:8080')
    : location.origin;

  const state = {
    user: readUser(),
    branding: {
      name: '玄凤小智',
      title: '玄凤小智管理后台',
      welcome: '欢迎使用玄凤小智后台管理面板',
      logoUrl: asset('logo.png'),
      homeLink: 'https://github.com/AnimeAIChat/xiaozhi-server-go/tree/main',
      copyright: '玄凤小智',
    },
    advancedTab: 'application',
  };

  const routeTitles = {
    login: '登录',
    welcome: '欢迎',
    agents: '智能体',
    agentConfig: '配置角色',
    devices: '管理设备',
    history: '历史对话',
    profile: '个人设置',
    dashboard: '仪表盘',
    adminSettings: '系统配置',
    providers: '模型供应商',
    unbindDevice: '设备解绑',
    advancedConfig: '高级配置',
    users: '用户管理',
    whiteList: '白名单',
  };

  function asset(path) {
    return `${assetBase}${path}`.replace(/\/{2,}/g, '/').replace(':/', '://');
  }

  function readUser() {
    try {
      return JSON.parse(localStorage.getItem('user') || 'null');
    } catch (_error) {
      return null;
    }
  }

  function saveUser(user) {
    state.user = user;
    localStorage.setItem('user', JSON.stringify(user));
  }

  function clearUser() {
    state.user = null;
    localStorage.removeItem('user');
  }

  function escapeHtml(value) {
    return String(value ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function prettyJson(value) {
    try {
      return JSON.stringify(value ?? {}, null, 2);
    } catch (_error) {
      return '{}';
    }
  }

  function parseJson(value, fallback = {}) {
    if (value == null || value === '') return fallback;
    if (typeof value === 'object') return value;
    try {
      return JSON.parse(value);
    } catch (_error) {
      return fallback;
    }
  }

  function byId(id) {
    return document.getElementById(id);
  }

  function qs(selector, root = document) {
    return root.querySelector(selector);
  }

  function qsa(selector, root = document) {
    return Array.from(root.querySelectorAll(selector));
  }

  function formatDate(value) {
    if (!value) return '暂无';
    let date;
    if (typeof value === 'number') {
      date = new Date(value * 1000);
    } else {
      date = new Date(value);
    }
    if (Number.isNaN(date.getTime()) || date.getFullYear() < 2020) return '暂无';
    return date.toLocaleString();
  }

  function formatPercent(value) {
    if (value == null || value === '') return '-';
    const number = Number(value);
    if (Number.isNaN(number)) return String(value);
    return `${Number(number.toFixed(2))}%`;
  }

  function normalizeList(payload) {
    if (Array.isArray(payload)) return payload;
    if (Array.isArray(payload?.data)) return payload.data;
    if (Array.isArray(payload?.data?.data)) return payload.data.data;
    if (Array.isArray(payload?.list)) return payload.list;
    return [];
  }

  function normalizeTablePayload(payload) {
    const data = normalizeList(payload);
    return {
      data,
      total: payload?.total ?? payload?.data?.total ?? data.length,
      success: payload?.success ?? payload?.status === 'ok' ?? true,
    };
  }

  function formValues(form) {
    const values = {};
    const data = new FormData(form);
    data.forEach((value, key) => {
      values[key] = value;
    });
    qsa('input[type="checkbox"]', form).forEach((input) => {
      values[input.name] = input.checked;
    });
    return values;
  }

  function toast(message, type = 'info') {
    const item = document.createElement('div');
    item.className = `toast ${type}`;
    item.textContent = message;
    toastRoot.appendChild(item);
    setTimeout(() => {
      item.remove();
    }, 3600);
  }

  function showError(error, fallback = '操作失败') {
    const message = error?.data?.message || error?.data?.error || error?.message || fallback;
    toast(message, 'error');
  }

  function openModal({ title, body, footer }) {
    modalRoot.innerHTML = `
      <div class="modal-backdrop" role="dialog" aria-modal="true">
        <div class="modal">
          <div class="modal-header">
            <h3>${escapeHtml(title)}</h3>
            <button class="btn ghost small" data-close-modal>关闭</button>
          </div>
          <div class="modal-body">${body}</div>
          <div class="modal-footer">${footer || ''}</div>
        </div>
      </div>
    `;
    qsa('[data-close-modal]', modalRoot).forEach((button) => {
      button.addEventListener('click', closeModal);
    });
  }

  function closeModal() {
    modalRoot.innerHTML = '';
  }

  function confirmAction(title, content, onConfirm) {
    openModal({
      title,
      body: `<p>${escapeHtml(content)}</p>`,
      footer: `
        <button class="btn" data-close-modal>取消</button>
        <button class="btn danger" id="confirm-ok">确认</button>
      `,
    });
    byId('confirm-ok').addEventListener('click', async () => {
      try {
        await onConfirm();
        closeModal();
      } catch (error) {
        showError(error);
      }
    });
  }

  async function request(path, options = {}) {
    const url = path.startsWith('http') ? path : `${apiBase}${path}`;
    const headers = new Headers(options.headers || {});
    headers.set('Accept', headers.get('Accept') || 'application/json');
    if (state.user?.token) {
      headers.set('Authorization', `Bearer ${state.user.token}`);
    }

    const init = {
      method: options.method || 'GET',
      headers,
      credentials: 'same-origin',
    };

    if (options.body instanceof FormData) {
      init.body = options.body;
    } else if (options.body !== undefined) {
      headers.set('Content-Type', headers.get('Content-Type') || 'application/json');
      init.body = typeof options.body === 'string' ? options.body : JSON.stringify(options.body);
    }

    let response;
    try {
      response = await fetch(url, init);
    } catch (error) {
      const networkError = new Error(`无法连接服务：${apiBase}`);
      networkError.cause = error;
      throw networkError;
    }

    const contentType = response.headers.get('content-type') || '';
    const payload = contentType.includes('application/json')
      ? await response.json().catch(() => ({}))
      : await response.text();

    if (!response.ok) {
      const error = new Error(payload?.message || payload?.error || response.statusText);
      error.status = response.status;
      error.data = payload;
      if (response.status === 401) {
        clearUser();
        navigate('/login');
      }
      throw error;
    }

    return payload;
  }

  function getRoutePath() {
    if (location.protocol === 'file:') {
      return (location.hash || '#/welcome').slice(1) || '/welcome';
    }
    return location.pathname === '/' ? '/welcome' : location.pathname;
  }

  function routeInfo(path = getRoutePath()) {
    const clean = path.replace(/\/+$/, '') || '/welcome';
    if (clean === '/login' || clean === '/console/login') return { key: 'login', public: true, path: '/login' };
    if (clean === '/welcome' || clean === '/') return { key: 'welcome', path: '/welcome' };
    if (clean === '/console/agents') return { key: 'agents', path: '/console/agents' };
    if (clean === '/console/settings') return { key: 'profile', path: '/console/settings' };
    let match = clean.match(/^\/console\/agents\/([^/]+)\/config$/);
    if (match) return { key: 'agentConfig', path: clean, params: { id: match[1] } };
    match = clean.match(/^\/console\/agents\/([^/]+)\/device$/);
    if (match) return { key: 'devices', path: clean, params: { id: match[1] } };
    match = clean.match(/^\/console\/agents\/([^/]+)\/history$/);
    if (match) return { key: 'history', path: clean, params: { id: match[1] } };
    if (clean === '/admin/dashboard') return { key: 'dashboard', path: clean };
    if (clean === '/admin/settings') return { key: 'adminSettings', path: clean };
    if (clean === '/admin/providers') return { key: 'providers', path: clean };
    if (clean === '/admin/unbindDevice') return { key: 'unbindDevice', path: clean };
    if (clean === '/admin/systemConfig') return { key: 'advancedConfig', path: clean };
    if (clean === '/admin/userManage') return { key: 'users', path: clean };
    if (clean === '/admin/whiteList') return { key: 'whiteList', path: clean };
    return { key: 'welcome', path: '/welcome' };
  }

  function navigate(path) {
    if (location.protocol === 'file:') {
      location.hash = path;
      render();
      return;
    }
    history.pushState({}, '', path);
    render();
  }

  function canSeeAdmin() {
    return state.user?.role === 'admin' || state.user?.role === 'observer';
  }

  function canWriteAdmin() {
    return state.user?.role === 'admin';
  }

  function shell(route) {
    const adminItems = canSeeAdmin()
      ? `
        <div class="nav-group">
          <div class="nav-title">管理页</div>
          ${navItem('/admin/dashboard', '仪表盘', route.path)}
          ${navItem('/admin/userManage', '用户管理', route.path)}
          ${navItem('/admin/settings', '系统配置', route.path)}
          ${navItem('/admin/providers', '模型供应商', route.path)}
          ${navItem('/admin/unbindDevice', '设备解绑', route.path)}
          ${navItem('/admin/systemConfig', '高级配置', route.path)}
          ${navItem('/admin/whiteList', '白名单', route.path)}
        </div>
      `
      : '';

    app.innerHTML = `
      <div class="layout">
        <aside class="sidebar" id="sidebar">
          <div class="brand">
            <img src="${escapeHtml(state.branding.logoUrl || asset('logo.png'))}" alt="">
            <span>${escapeHtml(state.branding.name || '玄凤小智')}</span>
          </div>
          <nav class="nav">
            <div class="nav-group">
              ${navItem('/welcome', '欢迎', route.path)}
            </div>
            <div class="nav-group">
              <div class="nav-title">控制台</div>
              ${navItem('/console/agents', '智能体', route.path)}
              ${navItem('/console/settings', '个人设置', route.path)}
            </div>
            ${adminItems}
          </nav>
        </aside>
        <section class="main">
          <header class="topbar">
            <div class="topbar-left">
              <button class="btn ghost mobile-menu" id="mobile-menu">菜单</button>
              <h1 class="page-title" id="page-title">${escapeHtml(routeTitles[route.key] || '')}</h1>
            </div>
            <div class="topbar-right">
              <a href="${escapeHtml(state.branding.homeLink || '#')}" target="_blank" rel="noreferrer">GitHub</a>
              <div class="user-menu">
                <button class="avatar-button" id="avatar-button">
                  <span class="avatar">${escapeHtml((state.user?.username || 'U').slice(0, 1).toUpperCase())}</span>
                  <span>${escapeHtml(state.user?.username || '用户')}</span>
                </button>
                <div class="dropdown" id="user-dropdown">
                  <button data-link="/console/settings">个人信息</button>
                  <button data-action="logout">退出登录</button>
                </div>
              </div>
            </div>
          </header>
          <main class="content" id="page"></main>
        </section>
      </div>
    `;

    byId('mobile-menu')?.addEventListener('click', () => {
      byId('sidebar')?.classList.toggle('open');
    });
    byId('avatar-button')?.addEventListener('click', (event) => {
      event.stopPropagation();
      byId('user-dropdown')?.classList.toggle('open');
    });
  }

  function navItem(path, label, currentPath) {
    const active = currentPath === path || (path !== '/welcome' && currentPath.startsWith(path));
    return `
      <a class="nav-item ${active ? 'active' : ''}" href="${path}" data-link="${path}">
        <span class="nav-icon">•</span>
        <span>${escapeHtml(label)}</span>
      </a>
    `;
  }

  function setTitle(title) {
    const fullTitle = title ? `${title} - ${state.branding.title}` : state.branding.title;
    document.title = fullTitle;
    const titleEl = byId('page-title');
    if (titleEl) titleEl.textContent = title;
  }

  function page() {
    return byId('page');
  }

  function pageHeader(title, subtitle = '', actions = '') {
    return `
      <div class="page-header">
        <div>
          <h2>${escapeHtml(title)}</h2>
          ${subtitle ? `<p>${escapeHtml(subtitle)}</p>` : ''}
        </div>
        <div class="toolbar">${actions}</div>
      </div>
    `;
  }

  function loadingCard(text = '正在加载...') {
    return `<div class="card"><div class="app-loading" style="min-height:180px"><div class="loading-mark"></div><div>${escapeHtml(text)}</div></div></div>`;
  }

  function emptyState(text, action = '') {
    return `<div class="empty">${escapeHtml(text)}${action ? `<div style="margin-top:16px">${action}</div>` : ''}</div>`;
  }

  async function render() {
    const route = routeInfo();
    state.user = readUser();

    if (route.key !== 'login' && !state.user) {
      renderLogin();
      return;
    }

    if (route.key === 'login') {
      renderLogin();
      return;
    }

    shell(route);
    setTitle(routeTitles[route.key] || '欢迎');

    const renderers = {
      welcome: renderWelcome,
      agents: renderAgents,
      agentConfig: renderAgentConfig,
      devices: renderDevices,
      history: renderHistory,
      profile: renderProfile,
      dashboard: renderDashboard,
      adminSettings: renderAdminSettings,
      providers: renderProviders,
      unbindDevice: renderUnbindDevice,
      advancedConfig: renderAdvancedConfig,
      users: renderUsers,
      whiteList: renderWhiteList,
    };

    try {
      await (renderers[route.key] || renderWelcome)(route);
    } catch (error) {
      page().innerHTML = `
        ${pageHeader(routeTitles[route.key] || '页面', '')}
        <div class="alert error">${escapeHtml(error.message || '页面加载失败')}</div>
      `;
    }
  }

  function renderLogin() {
    setTitle('登录');
    app.innerHTML = `
      <div class="login-page">
        <main class="login-main">
          <form class="login-card" id="login-form">
            <div class="login-brand">
              <img src="${escapeHtml(state.branding.logoUrl || asset('logo.png'))}" alt="">
              <h1 class="login-title">${escapeHtml(state.branding.title)}</h1>
              <div class="login-subtitle">账户密码登录</div>
            </div>
            <div class="form">
              <div class="field">
                <label for="login-username">用户名</label>
                <input id="login-username" name="username" autocomplete="username" placeholder="admin" required>
              </div>
              <div class="field">
                <label for="login-password">密码</label>
                <input id="login-password" name="password" type="password" autocomplete="current-password" placeholder="123456" required>
              </div>
              <label class="field checkbox">
                <input type="checkbox" name="autoLogin" checked>
                <span>自动登录</span>
              </label>
              <button class="btn primary" type="submit">登录</button>
              <div class="subtle">默认用户 admin，默认密码 123456，登录后请及时修改。</div>
            </div>
          </form>
        </main>
        <footer class="footer-note">© ${escapeHtml(state.branding.copyright || '玄凤小智')}</footer>
      </div>
    `;

    byId('login-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const values = formValues(event.currentTarget);
      try {
        const result = await request('/api/user/login', {
          method: 'POST',
          body: {
            username: values.username,
            password: values.password,
          },
        });
        if (result?.status === 'ok' && result?.data?.token) {
          saveUser(result.data);
          toast('登录成功', 'success');
          navigate('/welcome');
          return;
        }
        toast(result?.message || '登录失败', 'error');
      } catch (error) {
        showError(error, '登录失败');
      }
    });
  }

  async function renderWelcome() {
    page().innerHTML = `
      ${pageHeader('欢迎', state.branding.welcome)}
      <div class="card welcome-host" id="welcome-host">${loadingCard('正在加载欢迎页...')}</div>
    `;
    const host = byId('welcome-host');
    try {
      const response = await fetch(asset('welcome.html'), { cache: 'no-cache' });
      const text = await response.text();
      const styles = Array.from(text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/gi))
        .map((match) => `<style>${match[1]}</style>`)
        .join('');
      const bodyMatch = text.match(/<body[^>]*>([\s\S]*?)<\/body>/i);
      host.innerHTML = `${styles}${bodyMatch ? bodyMatch[1] : text}`;
    } catch (_error) {
      host.innerHTML = `
        <div class="empty">
          <h3>${escapeHtml(state.branding.welcome)}</h3>
          <p>欢迎使用开源版静态管理后台。</p>
        </div>
      `;
    }
  }

  async function renderDashboard() {
    page().innerHTML = `${pageHeader('仪表盘', '系统运行概览')}${loadingCard()}`;
    const data = await request('/api/user/summary').then((res) => res.data || {});
    const stats = [
      ['用户数量', data.totle_users ?? '-'],
      ['设备数量', data.totle_devices ?? '-'],
      ['智能体数量', data.totle_agents ?? '-'],
      ['在线用户数', data.online_users ?? '-'],
      ['对话设备数', data.session_devices ?? '-'],
      ['内存使用率', formatPercent(data.system_memory_use)],
      ['CPU 使用率', formatPercent(data.system_cpu_use)],
    ];
    page().innerHTML = `
      ${pageHeader('仪表盘', '系统运行概览')}
      <div class="grid stats-grid">
        ${stats.map(([title, value]) => `
          <div class="card">
            <div class="muted">${escapeHtml(title)}</div>
            <div class="stat-value">${escapeHtml(value)}</div>
          </div>
        `).join('')}
      </div>
    `;
  }

  async function getProvidersByType(type) {
    try {
      const payload = await request(`/api/user/providers/${type}`);
      return parseProviderMap(payload, type);
    } catch (_error) {
      return [];
    }
  }

  function parseProviderMap(payload, type) {
    const source = payload?.data || {};
    return Object.entries(source).map(([name, raw]) => {
      const config = parseJson(raw, {});
      return {
        id: `${type}:${name}`,
        name,
        type,
        status: 'active',
        config,
      };
    });
  }

  async function getModelProviders() {
    const groups = await Promise.all(['ASR', 'TTS', 'LLM', 'VLLLM'].map(getProvidersByType));
    return groups.flat();
  }

  function providerDisplay(provider) {
    const config = provider.config || {};
    if (provider.type === 'LLM' || provider.type === 'VLLLM') {
      return config.model_name || config.model || config.url || '';
    }
    if (provider.type === 'TTS') {
      return config.voice || config.type || '';
    }
    return config.type || config.url || config.output_dir || '';
  }

  function collectVoices(ttsProviders) {
    const voices = [];
    const seen = new Set();
    ttsProviders.forEach((provider) => {
      const config = provider.config || {};
      const raw = config.supported_voices || config.surported_voices || config.supportedVoices || [];
      if (Array.isArray(raw)) {
        raw.forEach((item) => {
          const value = typeof item === 'string' ? item : item?.name;
          if (!value || seen.has(value)) return;
          seen.add(value);
          voices.push({
            value,
            label: typeof item === 'string'
              ? item
              : (item.display_name_zh || item.display_name_cn || item.display_name || item.displayName || item.name),
            language: typeof item === 'object' && item.language ? item.language : '普通话',
          });
        });
      }
      if (config.voice && !seen.has(config.voice)) {
        seen.add(config.voice);
        voices.push({ value: config.voice, label: config.voice, language: '普通话' });
      }
    });
    return voices;
  }

  function voiceLabel(voices, value) {
    return voices.find((voice) => voice.value === value)?.label || value || '';
  }

  async function renderAgents() {
    page().innerHTML = `
      ${pageHeader('智能体', '管理角色、模型和设备绑定', `
        <input id="agent-search" placeholder="搜索智能体" style="min-height:32px;padding:6px 10px;border:1px solid var(--line);border-radius:6px">
        <button class="btn primary" id="create-agent">新建智能体</button>
        <button class="btn" id="agent-add-device">添加设备</button>
      `)}
      <div id="agents-body">${loadingCard('正在加载智能体列表...')}</div>
    `;

    byId('agent-add-device').addEventListener('click', () => toast('社区版本暂不支持绑定设备', 'warning'));
    byId('create-agent').addEventListener('click', () => openCreateAgentModal());

    const [agentPayload, ttsProviders] = await Promise.all([
      request('/api/user/agent/list'),
      getProvidersByType('TTS'),
    ]);
    const voices = collectVoices(ttsProviders);
    const agents = normalizeList(agentPayload);

    const draw = () => {
      const keyword = byId('agent-search')?.value?.trim().toLowerCase() || '';
      const filtered = agents.filter((agent) => !keyword || String(agent.name || '').toLowerCase().includes(keyword));
      byId('agents-body').innerHTML = filtered.length
        ? `<div class="grid agent-grid">${filtered.map((agent) => agentCard(agent, voices)).join('')}</div>`
        : emptyState('暂无智能体', '<button class="btn primary" id="create-first-agent">创建第一个智能体</button>');
      byId('create-first-agent')?.addEventListener('click', openCreateAgentModal);
    };

    byId('agent-search').addEventListener('input', draw);
    draw();
  }

  function agentCard(agent, voices) {
    const deviceCount = Array.isArray(agent.deviceIDs) ? agent.deviceIDs.length : 0;
    return `
      <div class="card">
        <div class="card-title">
          <h3>${escapeHtml(agent.name || '未命名智能体')}</h3>
          <button class="btn ghost danger small" data-delete-agent="${escapeHtml(agent.id)}">删除</button>
        </div>
        <div class="chip-row">
          <span class="tag blue">ID ${escapeHtml(agent.id)}</span>
          <span class="tag">${escapeHtml(agent.LLM || '-')}</span>
        </div>
        <div class="divider"></div>
        <div class="form">
          <div><span class="muted">角色音色：</span>${escapeHtml(voiceLabel(voices, agent.voice) || '-')}</div>
          <div><span class="muted">最近对话：</span>${escapeHtml(formatDate(agent.lastConversationAt))}</div>
          <div><span class="muted">设备数量：</span>${deviceCount}</div>
        </div>
        <div class="divider"></div>
        <div class="inline-actions">
          <button class="btn" data-link="/console/agents/${escapeHtml(agent.id)}/config">配置角色</button>
          <button class="btn" data-link="/console/agents/${escapeHtml(agent.id)}/history">历史对话</button>
          <button class="btn" data-link="/console/agents/${escapeHtml(agent.id)}/device">管理设备</button>
        </div>
      </div>
    `;
  }

  async function openCreateAgentModal() {
    openModal({
      title: '新建智能体',
      body: `
        <form class="form" id="create-agent-form">
          <div class="field">
            <label>智能体名称</label>
            <input name="name" placeholder="请输入智能体名称" required>
          </div>
        </form>
      `,
      footer: `
        <button class="btn" data-close-modal>取消</button>
        <button class="btn primary" id="create-agent-submit">创建</button>
      `,
    });
    byId('create-agent-submit').addEventListener('click', async () => {
      const form = byId('create-agent-form');
      if (!form.reportValidity()) return;
      const values = formValues(form);
      try {
        const [ttsProviders, llmProviders] = await Promise.all([
          getProvidersByType('TTS'),
          getProvidersByType('LLM'),
        ]);
        const voices = collectVoices(ttsProviders);
        const defaultVoice = voices[0]?.value || 'zh_female_wanwanxiaohe_moon_bigtts';
        const defaultVoiceName = voices[0]?.label || defaultVoice;
        const defaultLLM = llmProviders[0]?.name || 'Qwen';
        await request('/api/user/agent/create', {
          method: 'POST',
          body: {
            prompt: '',
            name: values.name,
            LLM: defaultLLM,
            language: '普通话',
            voice: defaultVoice,
            voiceName: defaultVoiceName,
            asrSpeed: 2,
            speakSpeed: 2,
            tone: 50,
          },
        });
        toast('智能体创建成功', 'success');
        closeModal();
        render();
      } catch (error) {
        showError(error, '创建失败');
      }
    });
  }

  async function renderAgentConfig(route) {
    const id = route.params.id;
    page().innerHTML = `
      ${pageHeader('配置角色', `智能体 ID：${id}`, `<button class="btn" data-link="/console/agents">返回</button>`)}
      <div id="agent-config-body">${loadingCard()}</div>
    `;
    const [agentPayload, ttsProviders, llmProviders, templates] = await Promise.all([
      request(`/api/user/agent/${id}`),
      getProvidersByType('TTS'),
      getProvidersByType('LLM'),
      fetch(asset('role_template.json'), { cache: 'no-cache' }).then((res) => res.json()).catch(() => []),
    ]);
    const agent = agentPayload.data || {};
    const voices = collectVoices(ttsProviders);
    if (agent.voice && !voices.some((voice) => voice.value === agent.voice)) {
      voices.unshift({ value: agent.voice, label: agent.voiceName || agent.voice, language: agent.language || '普通话' });
    }
    const languages = Array.from(new Set(voices.map((voice) => voice.language || '普通话')));

    byId('agent-config-body').innerHTML = `
      <form class="form" id="agent-config-form">
        <div class="card">
          <div class="card-title"><h3>角色模板</h3></div>
          <div class="chip-row">
            ${templates.map((tpl, index) => `
              <button type="button" class="btn small" data-template-index="${index}">${escapeHtml(tpl.name)}</button>
            `).join('') || '<span class="muted">暂无模板</span>'}
          </div>
        </div>
        <div class="card">
          <div class="card-title"><h3>基础配置</h3></div>
          <div class="form-grid">
            <div class="field">
              <label>助手昵称</label>
              <input name="assistantName" value="${escapeHtml(agent.name)}" required>
            </div>
            <div class="field">
              <label>对话语言</label>
              <select name="dialogLanguage" id="dialog-language">
                ${languages.map((language) => `<option value="${escapeHtml(language)}" ${language === agent.language ? 'selected' : ''}>${escapeHtml(language)}</option>`).join('')}
              </select>
            </div>
            <div class="field">
              <label>角色音色</label>
              <select name="voiceType" id="voice-type"></select>
            </div>
            <div class="field">
              <label>语言模型</label>
              <select name="languageModel" required>
                ${llmProviders.map((provider) => `
                  <option value="${escapeHtml(provider.name)}" ${provider.name === agent.LLM ? 'selected' : ''}>
                    ${escapeHtml(provider.name)}${providerDisplay(provider) ? ` (${escapeHtml(providerDisplay(provider))})` : ''}
                  </option>
                `).join('')}
              </select>
            </div>
          </div>
        </div>
        <div class="card">
          <div class="card-title"><h3>角色介绍</h3></div>
          <div class="field">
            <label>提示词</label>
            <textarea name="roleDescription" rows="8" required>${escapeHtml(agent.prompt || '')}</textarea>
          </div>
        </div>
        <div class="card">
          <div class="card-title"><h3>高级设置</h3></div>
          <div class="form-grid">
            <div class="field">
              <label>语音识别速度</label>
              <select name="recognitionSpeed">
                <option value="1" ${agent.asrSpeed === 1 ? 'selected' : ''}>耐心</option>
                <option value="2" ${agent.asrSpeed !== 1 && agent.asrSpeed !== 3 ? 'selected' : ''}>正常</option>
                <option value="3" ${agent.asrSpeed === 3 ? 'selected' : ''}>快速</option>
              </select>
            </div>
            <div class="field">
              <label>角色语速</label>
              <select name="speechSpeed">
                <option value="1" ${agent.speakSpeed === 1 ? 'selected' : ''}>慢速</option>
                <option value="2" ${agent.speakSpeed !== 1 && agent.speakSpeed !== 3 ? 'selected' : ''}>正常</option>
                <option value="3" ${agent.speakSpeed === 3 ? 'selected' : ''}>快速</option>
              </select>
            </div>
            <div class="field">
              <label>角色音调</label>
              <input name="pitchValue" type="range" min="0" max="100" value="${escapeHtml(agent.tone ?? 50)}">
            </div>
          </div>
        </div>
        <div class="inline-actions">
          <button type="button" class="btn" id="agent-reset">重置</button>
          <button type="submit" class="btn primary">保存</button>
        </div>
      </form>
    `;

    const syncVoiceOptions = () => {
      const selectedLanguage = byId('dialog-language').value;
      const filtered = voices.filter((voice) => (voice.language || '普通话') === selectedLanguage);
      const options = (filtered.length ? filtered : voices).map((voice) => `
        <option value="${escapeHtml(voice.value)}" ${voice.value === agent.voice ? 'selected' : ''}>${escapeHtml(voice.label)}</option>
      `).join('');
      byId('voice-type').innerHTML = options || `<option value="${escapeHtml(agent.voice || '')}">${escapeHtml(agent.voiceName || agent.voice || '默认音色')}</option>`;
    };
    syncVoiceOptions();
    byId('dialog-language').addEventListener('change', syncVoiceOptions);

    qsa('[data-template-index]').forEach((button) => {
      button.addEventListener('click', () => {
        const tpl = templates[Number(button.dataset.templateIndex)];
        const name = qs('[name="assistantName"]').value || '';
        qs('[name="roleDescription"]').value = String(tpl.prompt || '').replace(/\{\{\s*assistant_name\s*\}\}/g, name);
      });
    });

    byId('agent-reset').addEventListener('click', () => renderAgentConfig(route));
    byId('agent-config-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const values = formValues(event.currentTarget);
      const selectedVoice = voices.find((voice) => voice.value === values.voiceType);
      try {
        await request(`/api/user/agent/${id}`, {
          method: 'PUT',
          body: {
            prompt: values.roleDescription,
            name: values.assistantName,
            LLM: values.languageModel,
            language: values.dialogLanguage,
            voice: values.voiceType,
            voiceName: selectedVoice?.label || values.voiceType,
            asrSpeed: Number(values.recognitionSpeed),
            speakSpeed: Number(values.speechSpeed),
            tone: Number(values.pitchValue),
          },
        });
        toast('保存成功', 'success');
      } catch (error) {
        showError(error, '保存失败');
      }
    });
  }

  async function renderDevices(route) {
    const id = route.params.id;
    page().innerHTML = `
      ${pageHeader('管理设备', `智能体 ID：${id}`, `
        <button class="btn" data-link="/console/agents">返回</button>
        <button class="btn primary" id="add-device">添加设备</button>
      `)}
      <div id="devices-body">${loadingCard()}</div>
    `;
    byId('add-device').addEventListener('click', () => toast('社区版本暂不支持绑定设备', 'warning'));
    const payload = await request(`/api/user/device/list/${id}`);
    const devices = normalizeList(payload);
    byId('devices-body').innerHTML = devices.length
      ? `<div class="grid device-grid">${devices.map(deviceCard).join('')}</div>`
      : emptyState('暂无设备');
  }

  function deviceCard(device) {
    const deviceId = device.deviceId || device.DeviceID || '';
    return `
      <div class="card">
        <div class="card-title">
          <h3>${escapeHtml(device.name || '未命名设备')}</h3>
          <button class="btn ghost danger small" data-unbind-device="${escapeHtml(deviceId)}">解绑</button>
        </div>
        <div class="form">
          <div><span class="muted">MAC 地址：</span>${escapeHtml(maskDeviceId(deviceId))}</div>
          <div><span class="muted">固件版本：</span>${escapeHtml(device.version || '-')}</div>
          <div><span class="muted">最近活跃：</span>${escapeHtml(formatDate(device.lastActiveTimeV2 || device.lastActiveTime))}</div>
          <div><span class="muted">OTA 升级：</span>${device.ota ? '<span class="tag green">开启</span>' : '<span class="tag">关闭</span>'}</div>
        </div>
        <div class="divider"></div>
        <button class="btn small" data-copy="${escapeHtml(deviceId)}">复制 MAC</button>
      </div>
    `;
  }

  function maskDeviceId(value) {
    const parts = String(value || '').split(':');
    if (parts.length === 6) return `${parts[0]}:${parts[1]}:**:**:${parts[4]}:${parts[5]}`;
    return value || '-';
  }

  async function renderHistory(route) {
    const id = route.params.id;
    page().innerHTML = `
      ${pageHeader('历史对话', `智能体 ID：${id}`, `<button class="btn" data-link="/console/agents">返回</button>`)}
      <div class="history-layout">
        <div class="conversation-list" id="conversation-list">${loadingCard()}</div>
        <div class="message-panel" id="message-panel">
          <div class="empty">请选择一个对话</div>
        </div>
      </div>
    `;
    const payload = await request(`/api/user/agent/history_dialog_list/${id}`, { method: 'POST' });
    const conversations = normalizeList(payload);
    const list = byId('conversation-list');
    list.innerHTML = conversations.length
      ? conversations.map((item) => `
        <button class="conversation-item" data-conversation="${escapeHtml(item.id)}">
          <strong>${escapeHtml(formatDate(item.createdAt))}</strong>
          <div class="subtle">ID ${escapeHtml(item.id)} · ${escapeHtml(item.conversationId || '')}</div>
        </button>
      `).join('')
      : emptyState('暂无历史对话');
  }

  async function selectConversation(id) {
    qsa('.conversation-item').forEach((button) => {
      button.classList.toggle('active', button.dataset.conversation === String(id));
    });
    const panel = byId('message-panel');
    panel.innerHTML = loadingCard('正在加载对话详情...');
    const payload = await request(`/api/user/agent/history_dialog/${id}`);
    const dialog = payload?.data || {};
    const messages = parseJson(dialog.dialog, []);
    panel.innerHTML = `
      <div class="message-header">
        对话详情 - ${escapeHtml(formatDate(dialog.createdAt))}
        <button class="btn danger small" style="float:right" data-delete-dialog="${escapeHtml(id)}">删除对话</button>
      </div>
      <div class="message-list">
        ${Array.isArray(messages) && messages.length
          ? messages.map((message, index) => `
            <div class="message ${escapeHtml(message.role || 'assistant')}">
              <div class="subtle">${escapeHtml(message.role || `消息 ${index + 1}`)}</div>
              ${escapeHtml(message.content || '')}
            </div>
          `).join('')
          : emptyState('暂无消息')}
      </div>
    `;
  }

  async function renderProfile() {
    page().innerHTML = `${pageHeader('个人设置', '管理个人资料与登录密码')}${loadingCard()}`;
    const payload = await request('/api/user/profile');
    const user = payload.data || {};
    page().innerHTML = `
      ${pageHeader('个人设置', '管理个人资料与登录密码')}
      <div class="card">
        <form class="form" id="profile-form">
          <div class="form-grid">
            <div class="field">
              <label>用户名</label>
              <input value="${escapeHtml(user.username)}" disabled>
            </div>
            <div class="field">
              <label>昵称</label>
              <input name="nickname" value="${escapeHtml(user.nickname || user.username || '')}">
            </div>
            <div class="field">
              <label>邮箱</label>
              <input name="email" type="email" value="${escapeHtml(user.email || '')}">
            </div>
            <div class="field">
              <label>头像 URL</label>
              <input name="head_img" value="${escapeHtml(user.head_img || '')}">
            </div>
          </div>
          <div class="inline-actions">
            <button class="btn primary" type="submit">保存资料</button>
            <button class="btn" type="button" id="change-password">修改密码</button>
          </div>
        </form>
      </div>
    `;
    byId('profile-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      try {
        await request('/api/user/profile', { method: 'PUT', body: formValues(event.currentTarget) });
        toast('保存成功', 'success');
      } catch (error) {
        showError(error, '保存失败');
      }
    });
    byId('change-password').addEventListener('click', openPasswordModal);
  }

  function openPasswordModal() {
    openModal({
      title: '修改密码',
      body: `
        <form class="form" id="password-form">
          <div class="field"><label>旧密码</label><input name="old_password" type="password" required></div>
          <div class="field"><label>新密码</label><input name="new_password" type="password" minlength="6" required></div>
          <div class="field"><label>确认新密码</label><input name="confirm_password" type="password" minlength="6" required></div>
        </form>
      `,
      footer: `<button class="btn" data-close-modal>取消</button><button class="btn primary" id="password-submit">保存</button>`,
    });
    byId('password-submit').addEventListener('click', async () => {
      const form = byId('password-form');
      if (!form.reportValidity()) return;
      const values = formValues(form);
      if (values.new_password !== values.confirm_password) {
        toast('两次输入的新密码不一致', 'warning');
        return;
      }
      try {
        await request('/api/user/change-password', {
          method: 'POST',
          body: {
            old_password: values.old_password,
            new_password: values.new_password,
          },
        });
        toast('密码修改成功', 'success');
        closeModal();
      } catch (error) {
        showError(error, '密码修改失败');
      }
    });
  }

  async function renderAdminSettings() {
    page().innerHTML = `${pageHeader('系统配置', '模型选择与全局提示词')}${loadingCard()}`;
    const payload = await request('/api/admin/system');
    const config = payload.data || {};
    page().innerHTML = `
      ${pageHeader('系统配置', '模型选择与全局提示词')}
      <div class="card">
        <form class="form" id="admin-settings-form">
          <div class="form-grid">
            ${selectField('selectedASR', 'ASR 服务', config.selectedASR, config.asrList || [])}
            ${selectField('selectedTTS', 'TTS 服务', config.selectedTTS, config.ttsList || [])}
            ${selectField('selectedLLM', 'LLM 服务', config.selectedLLM, config.llmList || [])}
            ${selectField('selectedVLLLM', 'VLLLM 服务', config.selectedVLLLM, config.vllmList || [])}
          </div>
          <div class="field">
            <label>提示词设置</label>
            <textarea name="prompt" rows="10">${escapeHtml(config.prompt || '')}</textarea>
          </div>
          <div class="field">
            <label>快速回复词</label>
            <div class="chip-row" id="quick-reply-list">
              ${(config.quickReplyWords || []).map((word, index) => quickReplyTag(word, index)).join('')}
            </div>
            <div class="input-with-button">
              <input id="quick-reply-input" placeholder="输入新的快速回复词">
              <button type="button" class="btn" id="quick-reply-add">添加</button>
            </div>
          </div>
          <div class="inline-actions">
            <button type="button" class="btn" id="settings-reset">重置</button>
            <button type="submit" class="btn primary" ${canWriteAdmin() ? '' : 'disabled'}>保存配置</button>
          </div>
        </form>
      </div>
    `;
    let quickReplies = [...(config.quickReplyWords || [])];
    const drawQuickReplies = () => {
      byId('quick-reply-list').innerHTML = quickReplies.map((word, index) => quickReplyTag(word, index)).join('');
    };
    byId('quick-reply-add').addEventListener('click', () => {
      const input = byId('quick-reply-input');
      const value = input.value.trim();
      if (!value || quickReplies.includes(value)) return;
      quickReplies.push(value);
      input.value = '';
      drawQuickReplies();
    });
    byId('quick-reply-list').addEventListener('click', (event) => {
      const target = event.target.closest('[data-remove-quick]');
      if (!target) return;
      event.stopPropagation();
      quickReplies.splice(Number(target.dataset.removeQuick), 1);
      drawQuickReplies();
    });
    byId('settings-reset').addEventListener('click', renderAdminSettings);
    byId('admin-settings-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const values = formValues(event.currentTarget);
      const next = {
        selectedASR: values.selectedASR,
        selectedTTS: values.selectedTTS,
        selectedLLM: values.selectedLLM,
        selectedVLLLM: values.selectedVLLLM,
        prompt: values.prompt,
        quickReplyWords: quickReplies,
      };
      try {
        await request('/api/admin/system', { method: 'POST', body: { data: JSON.stringify(next) } });
        toast('配置保存成功', 'success');
      } catch (error) {
        showError(error, '保存失败');
      }
    });
  }

  function selectField(name, label, value, options) {
    return `
      <div class="field">
        <label>${escapeHtml(label)}</label>
        <select name="${escapeHtml(name)}">
          <option value="">请选择</option>
          ${options.map((option) => `<option value="${escapeHtml(option)}" ${option === value ? 'selected' : ''}>${escapeHtml(option)}</option>`).join('')}
        </select>
      </div>
    `;
  }

  function quickReplyTag(word, index) {
    return `<span class="tag blue">${escapeHtml(word)} <button type="button" class="btn link" data-remove-quick="${index}">×</button></span>`;
  }

  async function renderProviders() {
    page().innerHTML = `
      ${pageHeader('模型供应商', '管理 ASR、TTS、LLM 和 VLLLM 服务', `
        <input id="provider-search" placeholder="搜索供应商" style="min-height:32px;padding:6px 10px;border:1px solid var(--line);border-radius:6px">
        <select id="provider-filter" style="min-height:32px;padding:6px 10px;border:1px solid var(--line);border-radius:6px">
          <option value="all">所有类型</option>
          <option value="ASR">ASR</option>
          <option value="TTS">TTS</option>
          <option value="LLM">LLM</option>
          <option value="VLLLM">VLLLM</option>
        </select>
        <button class="btn primary" id="provider-add">添加供应商</button>
      `)}
      <div id="providers-body">${loadingCard()}</div>
    `;
    byId('provider-add').addEventListener('click', () => openProviderModal());
    const providers = await getModelProviders();
    const draw = () => {
      const keyword = byId('provider-search').value.trim().toLowerCase();
      const filter = byId('provider-filter').value;
      const filtered = providers.filter((provider) => {
        const text = `${provider.name} ${provider.type} ${providerDisplay(provider)}`.toLowerCase();
        return (filter === 'all' || provider.type === filter) && (!keyword || text.includes(keyword));
      });
      const stats = ['ASR', 'TTS', 'LLM', 'VLLLM'].map((type) => `${type}: ${providers.filter((p) => p.type === type).length}`).join('，');
      byId('providers-body').innerHTML = `
        <div class="card"><span class="muted">${escapeHtml(stats)}</span></div>
        ${filtered.length ? `<div class="grid provider-grid section">${filtered.map(providerCard).join('')}</div>` : emptyState('没有找到匹配的供应商')}
      `;
    };
    byId('provider-search').addEventListener('input', draw);
    byId('provider-filter').addEventListener('change', draw);
    draw();
  }

  function providerCard(provider) {
    const config = provider.config || {};
    const configRows = Object.entries(config).slice(0, 8).map(([key, value]) => `
      <div><span class="muted">${escapeHtml(key)}：</span>${escapeHtml(maskSecret(value))}</div>
    `).join('');
    return `
      <div class="card">
        <div class="card-title">
          <h3>${escapeHtml(provider.name)}</h3>
          <span class="tag blue">${escapeHtml(provider.type)}</span>
        </div>
        <div class="form">${configRows || '<span class="muted">暂无配置</span>'}</div>
        <div class="divider"></div>
        <div class="inline-actions">
          <button class="btn small" data-edit-provider="${escapeHtml(provider.type)}:${escapeHtml(provider.name)}">编辑</button>
          <button class="btn danger small" data-delete-provider="${escapeHtml(provider.type)}:${escapeHtml(provider.name)}">删除</button>
        </div>
      </div>
    `;
  }

  function maskSecret(value) {
    if (value == null) return '';
    const text = typeof value === 'object' ? JSON.stringify(value) : String(value);
    if (text.length > 32) return `${text.slice(0, 12)}...${text.slice(-6)}`;
    return text;
  }

  async function openProviderModal(provider = null) {
    openModal({
      title: provider ? '编辑供应商' : '添加供应商',
      body: `
        <form class="form" id="provider-form">
          <div class="form-grid">
            <div class="field">
              <label>服务分类</label>
              <select name="type" ${provider ? 'disabled' : ''}>
                ${['ASR', 'TTS', 'LLM', 'VLLLM'].map((type) => `<option value="${type}" ${provider?.type === type ? 'selected' : ''}>${type}</option>`).join('')}
              </select>
            </div>
            <div class="field">
              <label>供应商名称</label>
              <input name="name" value="${escapeHtml(provider?.name || '')}" ${provider ? 'disabled' : ''} required>
            </div>
            <div class="field">
              <label>具体类型</label>
              <input name="subType" value="${escapeHtml(provider?.config?.type || 'openai')}" required>
            </div>
          </div>
          <div class="field">
            <label>配置 JSON</label>
            <textarea class="codebox" name="configJson" spellcheck="false">${escapeHtml(prettyJson(provider?.config || { type: 'openai' }))}</textarea>
          </div>
        </form>
      `,
      footer: `<button class="btn" data-close-modal>取消</button><button class="btn primary" id="provider-submit">保存</button>`,
    });
    byId('provider-submit').addEventListener('click', async () => {
      const form = byId('provider-form');
      if (!form.reportValidity()) return;
      const values = formValues(form);
      const type = provider?.type || values.type;
      const name = provider?.name || values.name;
      const config = parseJson(values.configJson, null);
      if (!config) {
        toast('配置 JSON 格式不正确', 'warning');
        return;
      }
      config.type = values.subType || config.type;
      try {
        if (provider) {
          await request(`/api/user/providers/${encodeURIComponent(type)}/${encodeURIComponent(name)}`, {
            method: 'PUT',
            body: { data: config },
          });
        } else {
          await request('/api/user/providers/create', {
            method: 'POST',
            body: { type, name, data: config },
          });
        }
        toast('保存成功', 'success');
        closeModal();
        render();
      } catch (error) {
        showError(error, '保存失败');
      }
    });
  }

  async function renderUnbindDevice() {
    page().innerHTML = `
      ${pageHeader('设备解绑', '管理员可按设备 ID 解绑设备')}
      <div class="card">
        <form class="form" id="unbind-form">
          <div class="field">
            <label>设备 ID</label>
            <input name="deviceID" placeholder="请输入设备 ID" required>
          </div>
          <button class="btn danger" type="submit" ${canWriteAdmin() ? '' : 'disabled'}>解绑</button>
        </form>
      </div>
    `;
    byId('unbind-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const values = formValues(event.currentTarget);
      try {
        await request('/api/admin/system/device', { method: 'DELETE', body: { deviceID: values.deviceID } });
        toast('解绑成功', 'success');
      } catch (error) {
        showError(error, '解绑失败');
      }
    });
  }

  async function renderAdvancedConfig() {
    page().innerHTML = `
      ${pageHeader('高级配置', '管理系统运行参数')}
      <div class="card">
        <div class="tabs">
          ${[
            ['application', '应用配置'],
            ['transport', '传输配置'],
            ['web', 'Web 配置'],
            ['auth', '认证配置'],
            ['roles', '角色配置'],
            ['mcp-functions', '本地 MCP 功能'],
            ['exit-commands', '退出指令'],
            ['log', '日志配置'],
          ].map(([key, label]) => `<button class="tab ${state.advancedTab === key ? 'active' : ''}" data-advanced-tab="${key}">${label}</button>`).join('')}
        </div>
        <div id="advanced-body">${loadingCard()}</div>
      </div>
    `;
    await drawAdvancedTab(state.advancedTab);
  }

  async function drawAdvancedTab(tab) {
    state.advancedTab = tab;
    qsa('[data-advanced-tab]').forEach((button) => button.classList.toggle('active', button.dataset.advancedTab === tab));
    const body = byId('advanced-body');
    body.innerHTML = loadingCard();
    if (tab === 'application') return drawSimpleConfig('application', [
      ['quickReply', '快速回复', 'checkbox'],
      ['saveTtsAudio', '保存 TTS 音频', 'checkbox'],
      ['saveUserAudio', '保存用户音频文件', 'checkbox'],
    ]);
    if (tab === 'web') return drawSimpleConfig('web', [
      ['port', 'Web 服务端口', 'number'],
      ['staticDir', '静态目录', 'text'],
      ['websocket', 'WebSocket 地址', 'text'],
      ['visionUrl', '视觉服务 URL', 'text'],
      ['activateText', '激活文案', 'text'],
    ]);
    if (tab === 'auth') return drawSimpleConfig('auth', [
      ['token', '访问令牌', 'password'],
      ['expiry', '过期时间（小时）', 'number'],
    ]);
    if (tab === 'log') return drawSimpleConfig('log', [
      ['logLevel', '日志级别', 'select:debug,info,warn,error'],
      ['logDir', '日志目录', 'text'],
      ['logFile', '日志文件名', 'text'],
    ]);
    if (tab === 'transport') return drawTransportConfig();
    if (tab === 'roles') return drawCollectionConfig('roles', '角色', ['name', 'description', 'enabled']);
    if (tab === 'mcp-functions') return drawCollectionConfig('mcp-functions', '本地 MCP 功能', ['functionName', 'description', 'enabled']);
    if (tab === 'exit-commands') return drawCollectionConfig('exit-commands', '退出指令', ['command', 'enabled']);
  }

  async function drawSimpleConfig(type, fields) {
    const config = await request(`/api/admin/config/${type}`).then((res) => res.data || {});
    byId('advanced-body').innerHTML = `
      <form class="form" id="simple-config-form">
        <div class="form-grid">
          ${fields.map(([key, label, fieldType]) => simpleField(key, label, fieldType, config[key])).join('')}
        </div>
        <button class="btn primary" type="submit" ${canWriteAdmin() ? '' : 'disabled'}>保存配置</button>
      </form>
    `;
    byId('simple-config-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const values = formValues(event.currentTarget);
      fields.forEach(([key, _label, fieldType]) => {
        if (fieldType === 'number') values[key] = Number(values[key]);
      });
      try {
        await request(`/api/admin/config/${type}`, { method: 'PUT', body: values });
        toast('保存成功', 'success');
      } catch (error) {
        showError(error, '保存失败');
      }
    });
  }

  function simpleField(key, label, fieldType, value) {
    if (fieldType === 'checkbox') {
      return `<label class="field checkbox"><input type="checkbox" name="${escapeHtml(key)}" ${value ? 'checked' : ''}><span>${escapeHtml(label)}</span></label>`;
    }
    if (fieldType.startsWith('select:')) {
      const options = fieldType.slice(7).split(',');
      return `
        <div class="field">
          <label>${escapeHtml(label)}</label>
          <select name="${escapeHtml(key)}">
            ${options.map((option) => `<option value="${escapeHtml(option)}" ${option === value ? 'selected' : ''}>${escapeHtml(option)}</option>`).join('')}
          </select>
        </div>
      `;
    }
    return `
      <div class="field">
        <label>${escapeHtml(label)}</label>
        <input name="${escapeHtml(key)}" type="${fieldType}" value="${escapeHtml(value ?? '')}">
      </div>
    `;
  }

  async function drawTransportConfig() {
    const configs = await request('/api/admin/config/transport').then((res) => res.data || []);
    byId('advanced-body').innerHTML = `
      <form class="form" id="transport-form">
        <div class="field">
          <label>传输配置 JSON</label>
          <textarea class="codebox" name="transportJson" rows="18" spellcheck="false">${escapeHtml(prettyJson(configs))}</textarea>
        </div>
        <button class="btn primary" type="submit" ${canWriteAdmin() ? '' : 'disabled'}>保存配置</button>
      </form>
    `;
    byId('transport-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const values = formValues(event.currentTarget);
      const next = parseJson(values.transportJson, null);
      if (!Array.isArray(next)) {
        toast('传输配置必须是数组 JSON', 'warning');
        return;
      }
      try {
        await request('/api/admin/config/transport', { method: 'PUT', body: next });
        toast('保存成功', 'success');
      } catch (error) {
        showError(error, '保存失败');
      }
    });
  }

  async function drawCollectionConfig(endpoint, label, fields) {
    const payload = await request(`/api/admin/config/${endpoint}`);
    const rows = normalizeList(payload);
    byId('advanced-body').innerHTML = `
      <div class="toolbar" style="justify-content:space-between;margin-bottom:12px">
        <span class="muted">共 ${rows.length} 项</span>
        <button class="btn primary" id="collection-add" ${canWriteAdmin() ? '' : 'disabled'}>添加${escapeHtml(label)}</button>
      </div>
      <div class="table-wrap">
        <table>
          <thead><tr>${fields.map((field) => `<th>${escapeHtml(field)}</th>`).join('')}<th>操作</th></tr></thead>
          <tbody>
            ${rows.map((row) => `
              <tr>
                ${fields.map((field) => `<td>${field === 'enabled' ? (row[field] ? '<span class="tag green">启用</span>' : '<span class="tag">停用</span>') : escapeHtml(row[field] ?? '')}</td>`).join('')}
                <td>
                  <button class="btn small" data-edit-collection="${escapeHtml(endpoint)}:${escapeHtml(collectionId(row, endpoint))}">编辑</button>
                  <button class="btn danger small" data-delete-collection="${escapeHtml(endpoint)}:${escapeHtml(collectionId(row, endpoint))}">删除</button>
                </td>
              </tr>
            `).join('') || `<tr><td colspan="${fields.length + 1}">${emptyState(`暂无${label}`)}</td></tr>`}
          </tbody>
        </table>
      </div>
    `;
    byId('collection-add')?.addEventListener('click', () => openCollectionModal(endpoint, label, fields));
  }

  function collectionId(row, endpoint) {
    if (endpoint === 'roles') return row.name;
    if (endpoint === 'mcp-functions') return row.functionName;
    if (endpoint === 'exit-commands') return row.command;
    return row.id || row.name || '';
  }

  function openCollectionModal(endpoint, label, fields, row = null) {
    openModal({
      title: `${row ? '编辑' : '添加'}${label}`,
      body: `
        <form class="form" id="collection-form">
          ${fields.map((field) => field === 'enabled'
            ? `<label class="field checkbox"><input type="checkbox" name="enabled" ${row?.enabled ? 'checked' : ''}><span>启用</span></label>`
            : `<div class="field"><label>${escapeHtml(field)}</label><input name="${escapeHtml(field)}" value="${escapeHtml(row?.[field] ?? '')}" required></div>`
          ).join('')}
        </form>
      `,
      footer: `<button class="btn" data-close-modal>取消</button><button class="btn primary" id="collection-submit">保存</button>`,
    });
    byId('collection-submit').addEventListener('click', async () => {
      const form = byId('collection-form');
      if (!form.reportValidity()) return;
      const values = formValues(form);
      try {
        if (row) {
          await request(`/api/admin/config/${endpoint}/${encodeURIComponent(collectionId(row, endpoint))}`, { method: 'PUT', body: values });
        } else {
          await request(`/api/admin/config/${endpoint}`, { method: 'POST', body: values });
        }
        closeModal();
        toast('保存成功', 'success');
        drawAdvancedTab(endpoint);
      } catch (error) {
        showError(error, '保存失败');
      }
    });
  }

  async function renderUsers() {
    page().innerHTML = `
      ${pageHeader('用户管理', '查看和编辑用户状态与额度')}
      <div id="users-body">${loadingCard()}</div>
    `;
    try {
      const payload = await request('/api/admin/user/list?current=1&pageSize=50');
      const table = normalizeTablePayload(payload);
      byId('users-body').innerHTML = `
        <div class="card">
          <div class="table-wrap">
            <table>
              <thead><tr><th>ID</th><th>用户名</th><th>邮箱</th><th>状态</th><th>每日额度</th><th>创建时间</th><th>操作</th></tr></thead>
              <tbody>
                ${table.data.map((user) => `
                  <tr>
                    <td>${escapeHtml(user.id)}</td>
                    <td>${escapeHtml(user.username)}</td>
                    <td>${escapeHtml(user.email || '')}</td>
                    <td>${user.status === 1 ? '<span class="tag green">正常</span>' : '<span class="tag red">禁用</span>'}</td>
                    <td>${escapeHtml(user.profile?.quotaUsed ?? 0)} / ${escapeHtml(user.profile?.quotaTotal ?? '-')}</td>
                    <td>${escapeHtml(formatDate(user.createdAt))}</td>
                    <td><button class="btn small" data-edit-user="${escapeHtml(user.id)}">编辑</button></td>
                  </tr>
                `).join('') || `<tr><td colspan="7">${emptyState('暂无用户')}</td></tr>`}
              </tbody>
            </table>
          </div>
        </div>
      `;
    } catch (error) {
      byId('users-body').innerHTML = `<div class="alert warning">当前服务未提供用户管理接口，或当前账号无权限访问。</div>`;
    }
  }

  async function renderWhiteList() {
    page().innerHTML = `
      ${pageHeader('白名单', '管理允许访问的 MAC 地址', `
        <button class="btn primary" id="white-add">添加</button>
        <button class="btn" id="white-batch">批量添加</button>
      `)}
      <div id="white-body">${loadingCard()}</div>
    `;
    byId('white-add').addEventListener('click', openWhiteListModal);
    byId('white-batch').addEventListener('click', openWhiteListUpload);
    try {
      const payload = await request('/api/admin/system/whiteList?current=1&pageSize=50');
      const table = normalizeTablePayload(payload);
      byId('white-body').innerHTML = `
        <div class="card">
          <div class="table-wrap">
            <table>
              <thead><tr><th>MAC 地址</th><th>操作</th></tr></thead>
              <tbody>
                ${table.data.map((row) => `
                  <tr>
                    <td>${escapeHtml(row.mac || row.deviceId || '')}</td>
                    <td><button class="btn danger small" data-delete-white="${escapeHtml(row.mac || row.deviceId || '')}">删除</button></td>
                  </tr>
                `).join('') || `<tr><td colspan="2">${emptyState('暂无白名单')}</td></tr>`}
              </tbody>
            </table>
          </div>
        </div>
      `;
    } catch (_error) {
      byId('white-body').innerHTML = `<div class="alert warning">当前服务未提供白名单接口，或当前账号无权限访问。</div>`;
    }
  }

  function openWhiteListModal() {
    openModal({
      title: '添加白名单',
      body: `
        <form class="form" id="white-form">
          <div class="field">
            <label>MAC 地址</label>
            <input name="mac" pattern="^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$" placeholder="AA:BB:CC:DD:EE:FF" required>
          </div>
        </form>
      `,
      footer: `<button class="btn" data-close-modal>取消</button><button class="btn primary" id="white-submit">添加</button>`,
    });
    byId('white-submit').addEventListener('click', async () => {
      const form = byId('white-form');
      if (!form.reportValidity()) return;
      try {
        await request('/api/admin/system/whiteList/add', { method: 'POST', body: formValues(form) });
        closeModal();
        toast('添加成功', 'success');
        render();
      } catch (error) {
        showError(error, '添加失败');
      }
    });
  }

  function openWhiteListUpload() {
    openModal({
      title: '批量添加白名单',
      body: `
        <form class="form" id="white-upload-form">
          <div class="field">
            <label>CSV 文件</label>
            <input name="file" type="file" accept=".csv" required>
          </div>
        </form>
      `,
      footer: `<button class="btn" data-close-modal>取消</button><button class="btn primary" id="white-upload-submit">上传</button>`,
    });
    byId('white-upload-submit').addEventListener('click', async () => {
      const form = byId('white-upload-form');
      if (!form.reportValidity()) return;
      const file = qs('input[type="file"]', form).files[0];
      const data = new FormData();
      data.append('file', file);
      try {
        await request('/api/admin/system/whiteList/batchAdd', { method: 'POST', body: data });
        closeModal();
        toast('上传成功', 'success');
        render();
      } catch (error) {
        showError(error, '上传失败');
      }
    });
  }

  document.addEventListener('click', async (event) => {
    const target = event.target instanceof Element ? event.target : event.target.parentElement;
    if (!target) return;

    const link = target.closest('[data-link]');
    if (link) {
      event.preventDefault();
      navigate(link.dataset.link || link.getAttribute('href'));
      return;
    }

    if (target.closest('[data-action="logout"]')) {
      clearUser();
      navigate('/login');
      return;
    }

    if (!target.closest('.user-menu')) {
      byId('user-dropdown')?.classList.remove('open');
    }

    const copy = target.closest('[data-copy]');
    if (copy) {
      navigator.clipboard?.writeText(copy.dataset.copy);
      toast('已复制', 'success');
      return;
    }

    const removeQuick = target.closest('[data-remove-quick]');
    if (removeQuick) {
      removeQuick.closest('.tag')?.remove();
      return;
    }

    const deleteAgent = target.closest('[data-delete-agent]');
    if (deleteAgent) {
      confirmAction('删除智能体', '删除后相关配置和历史对话无法恢复，确认删除吗？', async () => {
        await request(`/api/user/agent/${deleteAgent.dataset.deleteAgent}`, { method: 'DELETE' });
        toast('删除成功', 'success');
        render();
      });
      return;
    }

    const unbind = target.closest('[data-unbind-device]');
    if (unbind) {
      confirmAction('解绑设备', '设备解绑后需要重新绑定，确认继续吗？', async () => {
        await request('/api/user/device', { method: 'DELETE', body: { deviceID: unbind.dataset.unbindDevice } });
        toast('解绑成功', 'success');
        render();
      });
      return;
    }

    const conversation = target.closest('[data-conversation]');
    if (conversation) {
      try {
        await selectConversation(conversation.dataset.conversation);
      } catch (error) {
        showError(error, '加载对话失败');
      }
      return;
    }

    const deleteDialog = target.closest('[data-delete-dialog]');
    if (deleteDialog) {
      confirmAction('删除对话', '删除后无法恢复，确认删除吗？', async () => {
        await request(`/api/user/agent/history_dialog/${deleteDialog.dataset.deleteDialog}`, { method: 'DELETE' });
        toast('删除成功', 'success');
        render();
      });
      return;
    }

    const editProvider = target.closest('[data-edit-provider]');
    if (editProvider) {
      const [type, ...nameParts] = editProvider.dataset.editProvider.split(':');
      const name = nameParts.join(':');
      const providers = await getModelProviders();
      const provider = providers.find((item) => item.type === type && item.name === name);
      openProviderModal(provider);
      return;
    }

    const deleteProvider = target.closest('[data-delete-provider]');
    if (deleteProvider) {
      const [type, ...nameParts] = deleteProvider.dataset.deleteProvider.split(':');
      const name = nameParts.join(':');
      confirmAction('删除供应商', `确认删除 ${type}/${name} 吗？`, async () => {
        await request(`/api/user/providers/${encodeURIComponent(type)}/${encodeURIComponent(name)}`, { method: 'DELETE' });
        toast('删除成功', 'success');
        render();
      });
      return;
    }

    const tab = target.closest('[data-advanced-tab]');
    if (tab) {
      await drawAdvancedTab(tab.dataset.advancedTab);
      return;
    }

    const editCollection = target.closest('[data-edit-collection]');
    if (editCollection) {
      const [endpoint, ...idParts] = editCollection.dataset.editCollection.split(':');
      const id = idParts.join(':');
      const payload = await request(`/api/admin/config/${endpoint}`);
      const rows = normalizeList(payload);
      const row = rows.find((item) => collectionId(item, endpoint) === id);
      const labelMap = { roles: '角色', 'mcp-functions': '本地 MCP 功能', 'exit-commands': '退出指令' };
      const fieldsMap = {
        roles: ['name', 'description', 'enabled'],
        'mcp-functions': ['functionName', 'description', 'enabled'],
        'exit-commands': ['command', 'enabled'],
      };
      openCollectionModal(endpoint, labelMap[endpoint], fieldsMap[endpoint], row);
      return;
    }

    const deleteCollection = target.closest('[data-delete-collection]');
    if (deleteCollection) {
      const [endpoint, ...idParts] = deleteCollection.dataset.deleteCollection.split(':');
      const id = idParts.join(':');
      confirmAction('删除配置项', '确认删除该配置项吗？', async () => {
        await request(`/api/admin/config/${endpoint}/${encodeURIComponent(id)}`, { method: 'DELETE' });
        toast('删除成功', 'success');
        drawAdvancedTab(endpoint);
      });
      return;
    }

    const deleteWhite = target.closest('[data-delete-white]');
    if (deleteWhite) {
      confirmAction('删除白名单', '确认删除该 MAC 地址吗？', async () => {
        await request('/api/admin/system/whiteList', {
          method: 'DELETE',
          body: { mac: deleteWhite.dataset.deleteWhite },
        });
        toast('删除成功', 'success');
        render();
      });
    }
  });

  window.addEventListener('popstate', render);
  window.addEventListener('hashchange', render);

  fetch(asset('branding.json'), { cache: 'no-cache' })
    .then((response) => response.ok ? response.json() : null)
    .then((branding) => {
      if (branding) {
        state.branding = {
          ...state.branding,
          ...branding,
          logoUrl: branding.logoUrl || state.branding.logoUrl,
        };
      }
    })
    .catch(() => null)
    .finally(render);
})();
