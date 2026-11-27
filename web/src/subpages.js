import { DATA } from "./data.js";
import { initEmblemCanvas } from "./home.js";
import {
  STAMP_PALETTE,
  TRAIL_COLORS,
  copyText,
  esc,
  makeRng,
  svgBitmapText,
  svgPixelIcon,
  validateRuleLines
} from "./graphics.js";

// Route 2: Core Modules & Capabilities (#/features)
export function renderFeatures(main, runtime) {
  document.title = "核心能力" + DATA.copy.siteTitleSuffix;
  let filterMode = "all";
  let selectedId = DATA.modules[0].id;

  main.innerHTML = `
    <section class="sub-sec" data-surface="lime">
      <div class="grid">
        <p class="col-1-3 t-micro">(核心能力矩阵)</p>
        <div class="col-5-12">
          <h1 class="t-h1" tabindex="-1">多组织出口、精准分流与流量账本，全链路协同。</h1>
          <p class="t-body" style="margin-top:16px;">点击任意模块像素图腾查看内部源码位置、协议机制与故障隔离设计。</p>
          <div style="margin-top:24px;display:flex;flex-wrap:wrap;gap:8px;" id="mod-filters">
            <button type="button" class="chip-toggle t-small" data-mf="all" aria-pressed="true">(全部 8 项)</button>
            <button type="button" class="chip-toggle t-small" data-mf="multi-org" aria-pressed="false">(多组织接入)</button>
            <button type="button" class="chip-toggle t-small" data-mf="routing" aria-pressed="false">(分流与回滚)</button>
            <button type="button" class="chip-toggle t-small" data-mf="coexist" aria-pressed="false">(共存与进程)</button>
            <button type="button" class="chip-toggle t-small" data-mf="stats" aria-pressed="false">(账本与桌面)</button>
          </div>
        </div>
        <div class="col-full" id="mods-grid-box" style="margin-top:32px;"></div>
        <div class="col-full" id="mod-detail-box" style="margin-top:48px;"></div>
      </div>
    </section>

    <section class="sec" data-surface="sand">
      <div class="grid">
        <p class="col-1-3 t-micro">(模块对照表)</p>
        <div class="col-5-12">
          <h2 class="t-h2">工程源码分布与底层协议对照</h2>
        </div>
        <div class="col-full" style="margin-top:24px;">
          <table class="data-table responsive-table">
            <thead>
              <tr>
                <th class="t-micro">模块</th>
                <th class="t-micro">核心模式</th>
                <th class="t-micro">对应源码位置</th>
                <th class="t-micro">协议与机制</th>
              </tr>
            </thead>
            <tbody>
              ${DATA.modules.map(m => `
                <tr>
                  <td data-label="模块" class="t-small fw-600">${esc(m.name)}</td>
                  <td data-label="核心模式" class="t-micro">${esc(m.mode)}</td>
                  <td data-label="源码位置" class="t-micro t-mono">${esc(m.pkgPath)}</td>
                  <td data-label="协议与机制" class="t-micro">${esc(m.protocol)}</td>
                </tr>
              `).join("")}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  `;

  const gridBox = document.getElementById("mods-grid-box");
  const detailBox = document.getElementById("mod-detail-box");
  const filterBtns = Array.from(document.querySelectorAll("#mod-filters [data-mf]"));

  function renderDetail() {
    const m = DATA.modules.find(x => x.id === selectedId) || DATA.modules[0];
    detailBox.innerHTML = `
      <div class="venue-details">
        <div style="display:flex;flex-wrap:wrap;justify-content:space-between;align-items:baseline;gap:12px;">
          <div>
            <p class="t-micro t-mono">(选中模块 · ${esc(m.flag)})</p>
            <h2 class="t-h2" style="margin-top:4px;">${esc(m.name)} — ${esc(m.mode)}</h2>
          </div>
          <div class="cmd-inline">
            <span>${esc(m.pkgPath)}</span>
            <button type="button" class="cmd-copy-btn" id="mod-copy-btn">复制路径</button>
          </div>
        </div>
        <p class="t-body" style="margin-top:16px;">${esc(m.highlight)}</p>
        <ul class="access-list t-small" style="margin-top:20px;">
          <li><strong>源码包路径：</strong><code class="t-mono">${esc(m.pkgPath)}</code></li>
          <li><strong>底层协议：</strong>${esc(m.protocol)}</li>
        </ul>
      </div>
    `;
    document.getElementById("mod-copy-btn").addEventListener("click", e => copyText(m.pkgPath, e.currentTarget));
  }

  function renderGrid() {
    const list = DATA.modules.filter(m => filterMode === "all" || m.category === filterMode);
    if (!list.some(x => x.id === selectedId) && list[0]) selectedId = list[0].id;
    gridBox.innerHTML = `<div class="speakers-preview-grid">
      ${list.map(m => `
        <figure class="speaker-card">
          <button type="button" class="speaker-card-link" style="width:100%;text-align:left;${m.id === selectedId ? "outline:3px solid var(--ink);outline-offset:4px;" : ""}" data-select-mod="${esc(m.id)}">
            <canvas class="portrait-canvas" data-modid="${esc(m.id)}" role="img" aria-label="${esc(m.name)}"></canvas>
            <figcaption class="speaker-card-figcap">
              <h3 class="t-lead speaker-card-name">${esc(m.name)}</h3>
              <p class="t-micro speaker-card-role">${esc(m.mode)}</p>
              <p class="t-micro t-mono speaker-card-talk">${esc(m.flag)}</p>
            </figcaption>
          </button>
        </figure>
      `).join("")}
    </div>`;

    list.forEach(m => {
      const cv = gridBox.querySelector(`canvas[data-modid="${m.id}"]`);
      if (cv) initEmblemCanvas(cv, m, runtime);
    });

    gridBox.querySelectorAll("[data-select-mod]").forEach(btn => {
      btn.addEventListener("click", () => {
        selectedId = btn.dataset.selectMod;
        renderGrid();
        renderDetail();
      });
    });
  }

  filterBtns.forEach(btn => {
    btn.addEventListener("click", () => {
      filterMode = btn.dataset.mf;
      filterBtns.forEach(b => b.setAttribute("aria-pressed", b.dataset.mf === filterMode ? "true" : "false"));
      renderGrid();
      renderDetail();
    });
  });

  renderGrid();
  renderDetail();
}

// Route 3: Architecture & Rollback (#/architecture)
export function renderArchitecture(main) {
  document.title = "架构与回滚" + DATA.copy.siteTitleSuffix;
  let selectedId = DATA.archModules[0].id;

  main.innerHTML = `
    <section class="sub-sec" data-surface="sand">
      <div class="grid">
        <p class="col-1-3 t-micro">(系统架构蓝图)</p>
        <div class="col-5-12">
          <h1 class="t-h1" tabindex="-1">前端与系统服务分离，切换出口自带防断网护栏。</h1>
          <p class="t-body" style="margin-top:16px;">点击架构图任意模块或右侧清单，查看桌面端、后台守护网关、tsnet 多组织发现与 sing-box 内核编排的边界。</p>
        </div>
      </div>

      <div class="grid venue-grid">
        <div class="col-1-8">
          <svg class="venue-svg" id="arch-svg" viewBox="0 0 40 24" shape-rendering="crispEdges" role="img" aria-label="SingTail 模块架构图"></svg>
          <p class="t-micro" style="margin-top:8px;">(Fig. 2 — SingTail 五层架构：桌面 UI / 托盘 → 本地控制网关 → 多组织发现 + 规则 + 内核编排 → sing-box TUN 与 SQLite。)</p>
        </div>
        <div class="col-9-12">
          <div class="venue-room-list" id="arch-list">
            ${DATA.archModules.map(m => `
              <button type="button" class="venue-room-btn t-small" data-mod="${esc(m.id)}" aria-pressed="${m.id === selectedId ? "true" : "false"}">
                <span class="fw-600">${esc(m.label)}</span>
                <span class="t-micro t-mono">${esc(m.chip)}</span>
              </button>
            `).join("")}
          </div>
          <div class="venue-details" id="arch-details"></div>
        </div>
      </div>
    </section>

    <section class="sec" data-surface="charcoal">
      <div class="grid">
        <p class="col-1-3 t-micro t-lime">(八重防护与防冲突机制)</p>
        <div class="col-5-12">
          <h2 class="t-h2 t-paper">切出口不怕断网，开着 Clash 也不撞网卡。</h2>
          <p class="t-body t-paper" style="margin-top:16px;opacity:0.86;">从公网 IP 实测回滚、外部代理进程放行，到坏规则集隔离与配置导出剥离私钥，每一条护栏都在代码里严格落地。</p>
        </div>
        <div class="col-full" style="margin-top:32px;">
          <table class="data-table responsive-table">
            <thead>
              <tr>
                <th class="t-micro">编号</th>
                <th class="t-micro">护栏机制</th>
                <th class="t-micro">生效规则 / 阈值</th>
                <th class="t-micro">保护目标</th>
              </tr>
            </thead>
            <tbody>
              ${DATA.protectionGuards.map(g => `
                <tr>
                  <td data-label="编号" class="t-micro t-mono t-lime">${esc(g.id.toUpperCase())}</td>
                  <td data-label="护栏机制" class="t-small fw-600 t-paper">${esc(g.name)}</td>
                  <td data-label="生效规则" class="t-micro t-mono t-cyan">${esc(g.sample)}</td>
                  <td data-label="保护目标" class="t-small t-paper">${esc(g.scope)}</td>
                </tr>
              `).join("")}
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <section class="sec" data-surface="lime">
      <div class="grid">
        <p class="col-1-3 t-micro">(本机数据与目录结构)</p>
        <div class="col-5-12">
          <h2 class="t-h2">单机目录清晰隔离，组织私钥永不离开本机。</h2>
          <div class="code-header" style="margin-top:24px;">
            <span class="t-micro fw-600">SingTail 安装与数据目录布局</span>
            <span class="t-micro t-mono">Local Data &amp; Built-in Core</span>
          </div>
          <pre class="code-block">SingTail/
├── singtail(.exe)                # 桌面客户端 (Wails 无边框壳 + 系统托盘)
├── singtail-service(.exe)        # 系统后台守护服务 (127.0.0.1:4189)
├── bin/sing-box(.exe)            # 内置官方 sing-box 1.14.3 内核
├── rules/*.srs                   # 内置 7 个基础规则集 (cn / !cn / ai / ads...)
└── data/
    ├── config.json               # 规则、订阅与偏好 (导出时剔除私钥)
    ├── traffic.db                # SQLite 多粒度流量账本 (48h + 日/月/组织)
    └── ts-state-&lt;org-id&gt;/        # 各 Tailscale 组织独立凭据目录</pre>
        </div>
      </div>
    </section>
  `;

  const svg = document.getElementById("arch-svg");
  const detailsEl = document.getElementById("arch-details");
  const listBtns = Array.from(document.querySelectorAll("#arch-list [data-mod]"));

  function paint() {
    let out = `<defs>${DATA.archModules.map(m => `<clipPath id="clip-${esc(m.id)}"><rect x="${m.x}" y="${m.y}" width="${m.w}" height="${m.h}"/></clipPath>`).join("")}</defs>`;
    out += `<rect x="0" y="0" width="40" height="24" fill="#161618" stroke="#111113" stroke-width="0.35"/>`;
    DATA.archModules.forEach(m => {
      const sel = m.id === selectedId;
      out += `<g class="venue-room-shape" data-mod="${esc(m.id)}" tabindex="0" role="button" aria-label="${esc(m.label)}">`;
      out += `<rect x="${m.x}" y="${m.y}" width="${m.w}" height="${m.h}" fill="${m.fill}" stroke="${sel ? "#FFFFFF" : "#111113"}" stroke-width="${sel ? "0.6" : "0.24"}"/>`;
      out += `<g clip-path="url(#clip-${esc(m.id)})">`;
      out += `<text x="${m.x + 0.7}" y="${m.y + 2.1}" font-family="Inter Tight, Noto Sans SC, sans-serif" font-size="1.08" font-weight="700" fill="${m.text}">${esc(m.label)}</text>`;
      out += `<text x="${m.x + 0.7}" y="${m.y + 3.7}" font-family="Geist Mono, monospace" font-size="0.72" fill="${m.text}" opacity="0.88">${esc(m.pkg)}</text>`;
      out += `<text x="${m.x + 0.7}" y="${m.y + m.h - 1.0}" font-family="Geist Mono, monospace" font-size="0.72" font-weight="600" fill="${m.text}">[${esc(m.chip)}]</text>`;
      out += `</g></g>`;
    });
    svg.innerHTML = out;

    svg.querySelectorAll("[data-mod]").forEach(el => {
      el.addEventListener("click", () => { selectedId = el.dataset.mod; paint(); });
      el.addEventListener("keydown", e => {
        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); selectedId = el.dataset.mod; paint(); }
      });
    });

    listBtns.forEach(b => b.setAttribute("aria-pressed", b.dataset.mod === selectedId ? "true" : "false"));

    const m = DATA.archModules.find(x => x.id === selectedId) || DATA.archModules[0];
    detailsEl.innerHTML = `
      <p class="t-micro t-mono">${esc(m.pkg)} · ${esc(m.chip)}</p>
      <h2 class="t-lead" style="margin-top:6px;">${esc(m.label)}</h2>
      <p class="t-small" style="margin-top:10px;">${esc(m.summary)}</p>
      <ul class="access-list t-small">
        ${m.details.map(d => `<li>${esc(d)}</li>`).join("")}
      </ul>
    `;
  }

  listBtns.forEach(b => b.addEventListener("click", () => { selectedId = b.dataset.mod; paint(); }));
  paint();
}

// Route 4: Daemon API & Release Artifacts (#/docs)
export function renderDocs(main) {
  document.title = "接口与产物" + DATA.copy.siteTitleSuffix;
  let query = "";
  let kindFilter = "all";
  let openId = "api-exit";

  main.innerHTML = `
    <section class="sub-sec" data-surface="sand">
      <div class="grid">
        <p class="col-1-3 t-micro">(接口与打包产物)</p>
        <div class="col-5-12">
          <h1 class="t-h1" tabindex="-1">本地控制接口与双平台正式产物清单。</h1>
          <p class="t-body" style="margin-top:16px;">守护服务监听 <code class="t-mono">127.0.0.1:4189</code> 并校验本机随机令牌；每次 Release 自动产出 Windows 与 macOS 共四份内置内核安装包。点击任意行展开示例。</p>
        </div>

        <div class="col-full" style="margin-top:32px;display:flex;flex-wrap:wrap;gap:16px;align-items:flex-end;">
          <div style="min-width:240px;flex:1;max-width:360px;">
            <label class="t-micro" for="docs-search">检索接口或产物名</label>
            <input class="text-input" style="margin-top:6px;background:#FFFFFF;" type="search" id="docs-search" placeholder="如：exit、rules、dmg、setup、snapshot">
          </div>
          <div style="display:flex;flex-wrap:wrap;gap:8px;" id="docs-kind-filters">
            <button type="button" class="chip-toggle t-small" data-kf="all" aria-pressed="true">(全部)</button>
            <button type="button" class="chip-toggle t-small" data-kf="Daemon API" aria-pressed="false">(Daemon API)</button>
            <button type="button" class="chip-toggle t-small" data-kf="Release 产物" aria-pressed="false">(Release 产物)</button>
          </div>
        </div>

        <p class="col-full t-micro" id="docs-count" style="margin-top:16px;"></p>
        <div class="col-full" id="docs-table-wrap"></div>
      </div>
    </section>
  `;

  const searchEl = document.getElementById("docs-search");
  const kindBtns = Array.from(document.querySelectorAll("#docs-kind-filters [data-kf]"));
  const countEl = document.getElementById("docs-count");
  const tableWrap = document.getElementById("docs-table-wrap");

  function drawTable() {
    const q = query.trim().toLowerCase();
    const list = DATA.docsItems.filter(it => {
      if (kindFilter !== "all" && it.kind !== kindFilter) return false;
      if (q && !(it.name + " " + it.params + " " + it.summary + " " + it.example).toLowerCase().includes(q)) return false;
      return true;
    });
    countEl.textContent = `显示 ${list.length} / ${DATA.docsItems.length} 项`;

    if (list.length === 0) {
      tableWrap.innerHTML = `<p class="t-lead" style="margin-top:32px;">没有匹配的接口或打包产物。</p>`;
      return;
    }

    tableWrap.innerHTML = `
      <table class="data-table responsive-table">
        <thead>
          <tr>
            <th class="t-micro">分类</th>
            <th class="t-micro">名称 / 路径</th>
            <th class="t-micro">参数 / 形态</th>
            <th class="t-micro">说明</th>
          </tr>
        </thead>
        <tbody>
          ${list.map(it => {
            const isOpen = openId === it.id;
            return `
              <tr class="talk-row" data-cat="${esc(it.cat)}" data-doc-id="${esc(it.id)}">
                <td data-label="分类" class="t-micro t-mono">${esc(it.kind)}</td>
                <td data-label="名称" class="t-small t-mono fw-600">${esc(it.name)}</td>
                <td data-label="参数" class="t-micro t-mono">${esc(it.params)}</td>
                <td data-label="说明" class="t-small">${esc(it.summary)}</td>
              </tr>
              ${isOpen ? `
                <tr class="talk-expand-row">
                  <td colspan="4">
                    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
                      <span class="t-micro fw-600">调用 / 使用示例 · ${esc(it.name)}</span>
                      <button type="button" class="cmd-copy-btn" data-copy-example="${esc(it.id)}">复制内容</button>
                    </div>
                    <pre class="code-block">${esc(it.example)}</pre>
                  </td>
                </tr>
              ` : ""}
            `;
          }).join("")}
        </tbody>
      </table>
    `;

    tableWrap.querySelectorAll("[data-doc-id]").forEach(row => {
      row.addEventListener("click", () => {
        openId = openId === row.dataset.docId ? null : row.dataset.docId;
        drawTable();
      });
    });
    tableWrap.querySelectorAll("[data-copy-example]").forEach(btn => {
      btn.addEventListener("click", e => {
        e.stopPropagation();
        const found = DATA.docsItems.find(x => x.id === btn.dataset.copyExample);
        if (found) copyText(found.example, btn);
      });
    });
  }

  searchEl.addEventListener("input", () => { query = searchEl.value; drawTable(); });
  kindBtns.forEach(btn => {
    btn.addEventListener("click", () => {
      kindFilter = btn.dataset.kf;
      kindBtns.forEach(b => b.setAttribute("aria-pressed", b.dataset.kf === kindFilter ? "true" : "false"));
      drawTable();
    });
  });
  drawTable();
}

// Route 5: Download & Install Wizard (#/download)
export function renderDownload(main) {
  document.title = "下载安装" + DATA.copy.siteTitleSuffix;
  const pkgs = {
    "win-setup": {
      label: "Windows 标准安装包 (.exe)",
      file: "SingTail-windows-amd64-setup.exe",
      desc: "适合日常主力机使用：自动安装至 Program Files\\SingTail、创建桌面快捷方式，并自动注册 SingTailService 开机静默自启，关窗口不断网。",
      steps: `# 1. 前往 GitHub Releases 下载并双击运行\nSingTail-windows-amd64-setup.exe\n\n# 2. 安装向导会自动完成以下工作：\n- 解压 singtail.exe、singtail-service.exe、bin/sing-box.exe 与 rules/*.srs\n- 注册并启动系统服务 SingTailService（开机免 UAC 静默驻留）\n- 在主界面点击「+ 挂载组织」即可网页授权或导入 ts-state 目录`
    },
    "win-zip": {
      label: "Windows 绿色便携包 (.zip)",
      file: "SingTail-windows-amd64-portable.zip",
      desc: "免安装解压即用：数据默认保存在程序同级目录的 data/ 下；双击 singtail.exe 即会自动拉起内置守护服务，需要开机自启时运行目录内的 install-service.bat。",
      steps: `# 1. 解压 SingTail-windows-amd64-portable.zip 到任意可写目录\n# 2. 直接双击启动桌面客户端：\nsingtail.exe\n\n# （可选）如需注册为 Windows 后台系统服务开机自启，请右键以管理员身份运行：\ninstall-service.bat`
    },
    "mac-arm64": {
      label: "macOS Apple Silicon (.dmg)",
      file: "SingTail-macos-arm64.dmg",
      desc: "适用于 M1 / M2 / M3 / M4 系列芯片 Mac：把 SingTail.app 拖入 Applications 即可运行，镜像内附 install-service.command 可注册后台守护。",
      steps: `# 1. 双击打开 SingTail-macos-arm64.dmg\n# 2. 将 SingTail.app 拖入右侧 Applications 快捷方式\n\n# （可选）如首次运行遇到未签名拦截，可在终端执行：\nxattr -dr com.apple.quarantine /Applications/SingTail.app\n\n# （可选）双击镜像内的 install-service.command 注册 macOS 后台守护`
    },
    "mac-amd64": {
      label: "macOS Intel (.dmg)",
      file: "SingTail-macos-amd64.dmg",
      desc: "适用于 Intel x86_64 芯片 Mac：同样内置 sing-box 1.14.3 与 7 个基础 .srs 规则包，支持网页授权与已有 ts-state 导入。",
      steps: `# 1. 双击打开 SingTail-macos-amd64.dmg\n# 2. 将 SingTail.app 拖入 Applications\n\n# （可选）解除系统下载隔离属性：\nxattr -dr com.apple.quarantine /Applications/SingTail.app`
    }
  };
  let activeKey = "win-setup";

  main.innerHTML = `
    <section class="sub-sec" data-surface="lime">
      <div class="grid">
        <p class="col-1-3 t-micro">(下载与安装)</p>
        <div class="col-5-12">
          <h1 class="t-h1" tabindex="-1">内置内核与规则包，下完就能用。</h1>
          <p class="t-body" style="margin-top:16px;">每次发布均在 GitHub Releases 同时提供 Windows 与 macOS 共四份正式产物，均内置 <code class="t-mono">sing-box 1.14.3</code> 与 7 个基础 <code class="t-mono">.srs</code> 规则包。</p>
        </div>

        <div class="col-1-5" style="margin-top:40px;">
          <p class="t-micro">(方式一 · 从 GitHub Releases 下载)</p>
          <h2 class="t-lead" style="margin-top:8px;">选择你的系统与安装包形态</h2>
          <p class="t-small" style="margin-top:12px;">右侧切换不同系统，查看对应产物文件名、服务安装方式与校验说明。</p>
          <div style="margin-top:20px;">
            <a class="btn-primary t-small" href="${esc(DATA.meta.releasesUrl)}" target="_blank" rel="noopener noreferrer"><span>前往 GitHub Releases 下载 ↗</span></a>
          </div>
        </div>
        <div class="col-7-12" style="margin-top:40px;">
          <div style="border:2px solid var(--ink);background:var(--sand);padding:20px;">
            <p class="t-micro fw-600">选择发布包类型</p>
            <div style="display:flex;flex-wrap:wrap;gap:8px;margin-top:10px;" id="pkg-tabs">
              ${Object.entries(pkgs).map(([k, v]) => `
                <button type="button" class="chip-toggle t-micro" data-pkg="${esc(k)}" aria-pressed="${k === activeKey ? "true" : "false"}">${esc(v.label)}</button>
              `).join("")}
            </div>
            <p class="t-small" id="pkg-desc" style="margin-top:16px;"></p>
            <div style="margin-top:20px;">
              <div class="code-header">
                <span class="t-micro fw-600 t-mono" id="pkg-filename"></span>
                <button type="button" class="cmd-copy-btn" id="pkg-copy-btn">复制步骤</button>
              </div>
              <pre class="code-block" id="pkg-steps"></pre>
            </div>
          </div>
        </div>

        <div class="col-1-5" style="margin-top:48px;">
          <p class="t-micro">(方式二 · 从源码编译)</p>
          <h2 class="t-lead" style="margin-top:8px;">Go + pnpm 零 CGO 交叉编译</h2>
          <p class="t-small" style="margin-top:12px;">需要 Go 1.23+、Node.js 22+ 与 pnpm。支持在 macOS / Linux 一键跑门禁并交叉编译 Windows 可执行文件。</p>
        </div>
        <div class="col-7-12" style="margin-top:48px;">
          <div class="code-header">
            <span class="t-micro fw-600">构建前端并编译双二进制</span>
            <button type="button" class="cmd-copy-btn" id="src-copy-btn">复制命令</button>
          </div>
          <pre class="code-block" id="src-build-code">git clone https://github.com/evepupil/SingTail.git
cd SingTail
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend build
rm -rf cmd/singtail-desktop/frontendDist && mkdir -p cmd/singtail-desktop/frontendDist
cp -R frontend/dist/. cmd/singtail-desktop/frontendDist/

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o singtail-service.exe ./cmd/singtail-service
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H windowsgui" -o singtail.exe ./cmd/singtail-desktop</pre>
        </div>
      </div>
    </section>

    <section class="sec" data-surface="sand" id="faq">
      <div class="grid">
        <p class="col-1-3 t-micro">(常见问题)</p>
        <div class="col-5-12">
          <h2 class="t-h2">关于 Clash 共存、多组织隔离与断网回滚的常见疑问。</h2>
          <div class="faq-list">
            ${DATA.faq.map((f, idx) => `
              <details class="faq-item"${idx === 0 ? " open" : ""}>
                <summary class="faq-summary t-lead">
                  <span>${esc(f.q)}</span>
                  <span class="faq-icon-plus">${svgPixelIcon("plus", 16)}</span>
                  <span class="faq-icon-minus">${svgPixelIcon("minus", 16)}</span>
                </summary>
                <p class="faq-answer t-body">${esc(f.a)}</p>
              </details>
            `).join("")}
          </div>
        </div>
      </div>
    </section>
  `;

  const descEl = document.getElementById("pkg-desc");
  const fnameEl = document.getElementById("pkg-filename");
  const stepsEl = document.getElementById("pkg-steps");
  const tabs = Array.from(document.querySelectorAll("#pkg-tabs [data-pkg]"));

  function paintPkg() {
    const p = pkgs[activeKey];
    tabs.forEach(b => b.setAttribute("aria-pressed", b.dataset.pkg === activeKey ? "true" : "false"));
    descEl.textContent = p.desc;
    fnameEl.textContent = p.file;
    stepsEl.textContent = p.steps;
  }

  tabs.forEach(b => b.addEventListener("click", () => {
    activeKey = b.dataset.pkg;
    paintPkg();
  }));
  document.getElementById("pkg-copy-btn").addEventListener("click", e => {
    copyText(stepsEl.textContent, e.currentTarget);
  });
  document.getElementById("src-copy-btn").addEventListener("click", e => {
    copyText(document.getElementById("src-build-code").textContent, e.currentTarget);
  });
  paintPkg();
}

// Route 6: Split-Route Rule Validator & Exit Node Card Builder (#/sandbox)
export function renderSandbox(main, runtime) {
  document.title = "分流沙箱" + DATA.copy.siteTitleSuffix;

  const sampleRules = `# 粘贴 Clash / Surge / 进程规则文本，实时校验 SingTail 解析结果
DOMAIN-SUFFIX,openai.com,EXIT
DOMAIN-SUFFIX,github.com,EXIT
PROCESS-NAME,Code.exe,DIRECT
PROCESS-NAME,verge-mihomo.exe,DIRECT
DOMAIN-KEYWORD,internal-corp,BACKUP
IP-CIDR,8.128.0.0/10,DIRECT
RULE-SET,geosite-category-ads-all,REJECT`;

  const nodeCard = {
    org: "Personal Tailnet",
    node: "hk-cloud-01",
    ip: "43.153.82.19",
    link: "P2P 直连 · 34 ms"
  };

  let grid = runtime.state.stampGrid.split("");
  let currentColor = "2";
  let painting = false;

  main.innerHTML = `
    <section class="sub-sec" data-surface="charcoal">
      <div class="grid">
        <p class="col-1-3 t-micro t-lime">(分流规则沙箱 &amp; 节点工坊)</p>
        <div class="col-5-12">
          <h1 class="t-h1 t-paper" tabindex="-1">在线测试规则文本校验，绘制专属出口节点徽章。</h1>
          <p class="t-body t-paper" style="margin-top:16px;opacity:0.86;">在左侧粘贴 Clash / Surge / 进程规则文本实时校验语法，右侧可同步生成导出配置预览与可下载的 SingTail 节点像素名片。</p>
        </div>
      </div>

      <div class="grid badge-layout">
        <div class="col-1-6">
          <div style="display:flex;flex-direction:column;gap:14px;">
            <div>
              <div style="display:flex;justify-content:space-between;align-items:center;">
                <label class="t-micro t-lime" for="sb-rules">分流规则文本 (支持批量导入格式)</label>
                <button type="button" class="btn-text t-micro u-anim" id="sb-inject-bad">(注入错误行测试拦截)</button>
              </div>
              <textarea class="textarea-input t-mono" style="margin-top:6px;min-height:190px;font-size:0.84rem;" id="sb-rules">${esc(sampleRules)}</textarea>
            </div>
            <div id="sb-guard-status"></div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
              <div>
                <label class="t-micro t-lime" for="sb-org">所属组织 (Tailnet)</label>
                <input class="text-input" style="margin-top:6px;" id="sb-org" value="${esc(nodeCard.org)}">
              </div>
              <div>
                <label class="t-micro t-lime" for="sb-node">出口节点名 (Exit Node)</label>
                <input class="text-input" style="margin-top:6px;" id="sb-node" value="${esc(nodeCard.node)}">
              </div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
              <div>
                <label class="t-micro t-lime" for="sb-ip">实测公网出口 IP</label>
                <input class="text-input t-mono" style="margin-top:6px;" id="sb-ip" value="${esc(nodeCard.ip)}">
              </div>
              <div>
                <label class="t-micro t-lime" for="sb-link">链路与延迟</label>
                <input class="text-input t-mono" style="margin-top:6px;" id="sb-link" value="${esc(nodeCard.link)}">
              </div>
            </div>
          </div>

          <div style="margin-top:28px;padding-top:24px;border-top:1px solid var(--hairline-dark);">
            <p class="t-micro t-lime">(可选 · 16×16 节点矩阵徽章画板)</p>
            <div style="display:flex;flex-wrap:wrap;gap:20px;align-items:flex-start;margin-top:12px;">
              <canvas class="badge-editor-canvas" id="stamp-canvas" style="max-width:224px;" role="img" aria-label="16×16 节点矩阵像素画板"></canvas>
              <div style="flex:1;min-width:160px;">
                <div class="palette-group" id="stamp-palette" style="margin-top:0;">
                  ${STAMP_PALETTE.map(p => `
                    <button type="button" class="palette-swatch" data-col="${p.id}" aria-checked="${p.id === currentColor ? "true" : "false"}" style="background:${p.hex}" title="${p.name}"></button>
                  `).join("")}
                </div>
                <div style="display:flex;flex-wrap:wrap;gap:8px;margin-top:14px;">
                  <button type="button" class="chip-toggle t-micro" id="stamp-random">(随机节点纹)</button>
                  <button type="button" class="chip-toggle t-micro" id="stamp-clear">(清空)</button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="col-7-12">
          <div class="code-header">
            <span class="t-micro fw-600" id="sb-filename-label">singtail-rules-preview.json</span>
            <span class="t-micro t-mono" id="sb-count-label">0 rules</span>
          </div>
          <pre class="code-block" id="sb-json-preview" style="min-height:240px;"></pre>

          <div style="margin-top:20px;display:flex;flex-wrap:wrap;gap:12px;">
            <button type="button" class="btn-primary t-small" id="sb-dl-json"><span>下载校验通过的规则清单 (.json)</span></button>
            <button type="button" class="btn-outline t-small" id="sb-dl-svg">导出出口节点徽章卡 (.svg)</button>
          </div>

          <svg id="sb-card-svg" viewBox="0 0 420 220" shape-rendering="crispEdges" style="display:block;width:100%;max-width:460px;height:auto;margin-top:20px;border:1px solid var(--lime);"></svg>
        </div>
      </div>
    </section>
  `;

  const rulesEl = document.getElementById("sb-rules");
  const orgEl = document.getElementById("sb-org");
  const nodeEl = document.getElementById("sb-node");
  const ipEl = document.getElementById("sb-ip");
  const linkEl = document.getElementById("sb-link");
  const guardEl = document.getElementById("sb-guard-status");
  const jsonPrevEl = document.getElementById("sb-json-preview");
  const countLabelEl = document.getElementById("sb-count-label");
  const cardSvg = document.getElementById("sb-card-svg");
  const stampCanvas = document.getElementById("stamp-canvas");

  function renderStampCanvas() {
    const size = 224;
    const cell = 14;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    stampCanvas.width = size * dpr;
    stampCanvas.height = size * dpr;
    const ctx = stampCanvas.getContext("2d");
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = DATA.tokens.rail;
    ctx.fillRect(0, 0, size, size);
    for (let r = 0; r < 16; r++) {
      for (let c = 0; c < 16; c++) {
        const palObj = STAMP_PALETTE[Number(grid[r * 16 + c])] || STAMP_PALETTE[0];
        ctx.fillStyle = palObj.hex;
        ctx.fillRect(c * cell + 1, r * cell + 1, cell - 1, cell - 1);
      }
    }
  }

  function updateAll() {
    nodeCard.org = orgEl.value.trim() || "Personal Tailnet";
    nodeCard.node = nodeEl.value.trim() || "hk-cloud-01";
    nodeCard.ip = ipEl.value.trim() || "43.153.82.19";
    nodeCard.link = linkEl.value.trim() || "P2P 直连";

    const res = validateRuleLines(rulesEl.value);
    countLabelEl.textContent = `${res.parsed.length} 条有效规则`;

    const exportObj = {
      version: 1,
      sanitized: true,
      note: "不包含任何 ts-state 设备私钥",
      rules: res.parsed.map(r => ({ type: r.kind, value: r.value, action: r.action, enabled: true }))
    };
    jsonPrevEl.textContent = JSON.stringify(exportObj, null, 2);

    if (res.errors.length > 0) {
      guardEl.innerHTML = `<div class="guard-banner t-small">规则校验未完全通过（已跳过 ${res.errors.length} 行错误）：<br>${esc(res.errors[0])}</div>`;
    } else {
      guardEl.innerHTML = `<div class="guard-banner is-ok t-small">规则校验全部通过：共 ${res.parsed.length} 条规则可直接导入 SingTail 置顶生效。</div>`;
    }

    let stampRects = "";
    for (let r = 0; r < 16; r++) {
      for (let c = 0; c < 16; c++) {
        const palObj = STAMP_PALETTE[Number(grid[r * 16 + c])] || STAMP_PALETTE[0];
        stampRects += `<rect x="${20 + c * 7}" y="${58 + r * 7}" width="7" height="7" fill="${palObj.hex}"/>`;
      }
    }
    let strip = "";
    for (let i = 0; i < 21; i++) {
      strip += `<rect x="${i * 20}" y="204" width="20" height="16" fill="${TRAIL_COLORS[i % TRAIL_COLORS.length]}"/>`;
    }
    cardSvg.innerHTML = `
      <rect x="0" y="0" width="420" height="220" fill="${DATA.tokens.charcoal}"/>
      <rect x="0" y="0" width="420" height="38" fill="${DATA.tokens.rail}"/>
      <text x="20" y="24" font-family="Geist Mono, monospace" font-size="13" font-weight="700" fill="${DATA.tokens.lime}">SINGTAIL // EXIT NODE</text>
      <text x="404" y="24" text-anchor="end" font-family="Geist Mono, monospace" font-size="11" fill="${DATA.tokens.cyan}">${esc(nodeCard.link.slice(0, 22))}</text>
      ${stampRects}
      <text x="150" y="84" font-family="Inter Tight, Noto Sans SC, sans-serif" font-size="20" font-weight="700" fill="${DATA.tokens.paper}">${esc(nodeCard.node.slice(0, 18))}</text>
      <text x="150" y="110" font-family="Inter Tight, Noto Sans SC, sans-serif" font-size="12" fill="${DATA.tokens.lime}">${esc(nodeCard.org.slice(0, 26))}</text>
      <text x="150" y="140" font-family="Geist Mono, monospace" font-size="12" fill="${DATA.tokens.cyan}">PUBLIC IP: ${esc(nodeCard.ip.slice(0, 20))}</text>
      <text x="150" y="164" font-family="Geist Mono, monospace" font-size="11" fill="${DATA.tokens.green}">RULES: ${res.parsed.length} VALID · ${res.errors.length} ERR</text>
      ${strip}
    `;
  }

  [rulesEl, orgEl, nodeEl, ipEl, linkEl].forEach(el => el.addEventListener("input", updateAll));

  document.getElementById("sb-inject-bad").addEventListener("click", () => {
    if (!rulesEl.value.includes("UNKNOWN-TYPE")) {
      rulesEl.value += "\nUNKNOWN-TYPE,bad.example.com,EXIT";
      updateAll();
    }
  });

  function paintAt(e) {
    const rect = stampCanvas.getBoundingClientRect();
    const c = Math.min(15, Math.max(0, Math.floor(((e.clientX - rect.left) / rect.width) * 16)));
    const r = Math.min(15, Math.max(0, Math.floor(((e.clientY - rect.top) / rect.height) * 16)));
    grid[r * 16 + c] = currentColor;
    runtime.state.stampGrid = grid.join("");
    renderStampCanvas();
    updateAll();
  }
  stampCanvas.addEventListener("pointerdown", e => { e.preventDefault(); painting = true; paintAt(e); });
  stampCanvas.addEventListener("pointermove", e => { if (painting) paintAt(e); });
  window.addEventListener("pointerup", () => { painting = false; });

  document.querySelectorAll("#stamp-palette [data-col]").forEach(btn => {
    btn.addEventListener("click", () => {
      currentColor = btn.dataset.col;
      document.querySelectorAll("#stamp-palette [data-col]").forEach(b => b.setAttribute("aria-checked", b.dataset.col === currentColor ? "true" : "false"));
    });
  });

  document.getElementById("stamp-random").addEventListener("click", () => {
    const rng = makeRng(Date.now());
    const accent = String(2 + Math.floor(rng() * 4));
    const g = new Array(256).fill("7");
    for (let r = 2; r <= 13; r++) {
      for (let c = 0; c < 8; c++) {
        if (rng() < 0.42) { g[r * 16 + c] = accent; g[r * 16 + (15 - c)] = accent; }
      }
    }
    grid = g;
    runtime.state.stampGrid = grid.join("");
    renderStampCanvas();
    updateAll();
  });

  document.getElementById("stamp-clear").addEventListener("click", () => {
    grid = new Array(256).fill("7");
    runtime.state.stampGrid = grid.join("");
    renderStampCanvas();
    updateAll();
  });

  document.getElementById("sb-dl-json").addEventListener("click", () => {
    const blob = new Blob([jsonPrevEl.textContent], { type: "application/json;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "singtail-rules-sanitized.json";
    document.body.appendChild(a);
    a.click();
    a.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  });

  document.getElementById("sb-dl-svg").addEventListener("click", () => {
    const ser = new XMLSerializer();
    const clone = cardSvg.cloneNode(true);
    clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    const blob = new Blob([`<?xml version="1.0" encoding="UTF-8"?>\n` + ser.serializeToString(clone)], { type: "image/svg+xml;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "singtail-exit-card.svg";
    document.body.appendChild(a);
    a.click();
    a.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  });

  renderStampCanvas();
  updateAll();
}

// 404 Route
export function renderNotFound(main) {
  document.title = "页面未找到" + DATA.copy.siteTitleSuffix;
  const ch = window.innerWidth >= 768 ? 14 : 8;
  main.innerHTML = `
    <section class="sub-sec" data-surface="lime">
      <div class="grid">
        <div class="col-full">
          ${svgBitmapText("404", ch, DATA.tokens.ink, { className: "notfound-bitmap", ariaHidden: true, bold: true })}
          <h1 class="t-h1" tabindex="-1">未找到对应页面路由。</h1>
          <div style="margin-top:32px;">
            <a class="btn-text t-lead u-anim" href="#/">(返回 SingTail 首页)</a>
          </div>
        </div>
      </div>
    </section>
  `;
}
