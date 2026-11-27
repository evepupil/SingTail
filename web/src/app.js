import { DATA } from "./data.js";
import { TRACK_COLORS, TRAIL_COLORS, esc, makeRng, svgBitmapText } from "./graphics.js";
import { renderHome } from "./home.js";
import {
  renderArchitecture,
  renderDocs,
  renderDownload,
  renderFeatures,
  renderNotFound,
  renderSandbox
} from "./subpages.js";

function defaultNodeGrid() {
  // 16x16 pixel version of SingTail's 3x3 Exit-Node Matrix Logo
  const g = new Array(256).fill("7");
  const fillBox = (x0, y0, sz, col) => {
    for (let y = y0; y < y0 + sz; y++) {
      for (let x = x0; x < x0 + sz; x++) {
        g[y * 16 + x] = col;
      }
    }
  };
  // Inactive muted squares
  [[3, 3], [7, 3], [3, 7], [11, 7], [11, 11]].forEach(([x, y]) => fillBox(x, y, 2, "0"));
  // Connectors
  fillBox(5, 11, 2, "1");
  fillBox(7, 9, 2, "1");
  g[6 * 16 + 9] = "2";
  g[5 * 16 + 10] = "2";
  // Active path & top-right exit node
  [[3, 11], [7, 11], [7, 7]].forEach(([x, y]) => fillBox(x, y, 2, "1"));
  fillBox(11, 3, 2, "2");
  return g.join("");
}

function brandIconSVG() {
  return `<svg class="logo-icon-svg" viewBox="0 0 20 20" fill="none" aria-hidden="true">
    <rect x="0.5" y="0.5" width="19" height="19" rx="2" fill="#121214" stroke="#36363D" stroke-width="1"/>
    <rect x="2.5" y="2.5" width="3.5" height="3.5" fill="#3E3E46"/>
    <rect x="8.25" y="2.5" width="3.5" height="3.5" fill="#3E3E46"/>
    <rect x="2.5" y="8.25" width="3.5" height="3.5" fill="#3E3E46"/>
    <rect x="14" y="8.25" width="3.5" height="3.5" fill="#3E3E46"/>
    <rect x="14" y="14" width="3.5" height="3.5" fill="#3E3E46"/>
    <path d="M6 15.75H8.25M10 14V11.75" stroke="#ECECEE" stroke-width="1.5"/>
    <path d="M11.5 8.5L14.25 5.75" stroke="#CEFF00" stroke-width="1.6" stroke-linecap="square"/>
    <rect x="2.5" y="14" width="3.5" height="3.5" fill="#ECECEE"/>
    <rect x="8.25" y="14" width="3.5" height="3.5" fill="#ECECEE"/>
    <rect x="8.25" y="8.25" width="3.5" height="3.5" fill="#ECECEE"/>
    <rect x="14" y="2.5" width="3.5" height="3.5" fill="#CEFF00"/>
  </svg>`;
}

function storageGet(key, fallback) {
  try {
    const raw = window.localStorage.getItem(key);
    return raw === null ? fallback : JSON.parse(raw);
  } catch {
    return fallback;
  }
}

function storageSet(key, val) {
  try {
    window.localStorage.setItem(key, JSON.stringify(val));
  } catch {
    // ignore
  }
}

const prefersReduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const savedMotion = storageGet("singtail.motion", null);

const state = {
  motionOn: savedMotion === null ? !prefersReduce : Boolean(savedMotion),
  stampGrid: defaultNodeGrid(),
  routeCleanup: []
};

const motion = (() => {
  const subs = new Set();
  let rafId = 0;

  function tick(now) {
    rafId = 0;
    if (document.visibilityState === "hidden" || subs.size === 0) return;
    for (const fn of Array.from(subs)) {
      try { fn(now); } catch { subs.delete(fn); }
    }
    if (subs.size > 0 && document.visibilityState !== "hidden") {
      rafId = window.requestAnimationFrame(tick);
    }
  }

  function ensure() {
    if (!rafId && subs.size > 0 && document.visibilityState !== "hidden") {
      rafId = window.requestAnimationFrame(tick);
    }
  }

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") {
      if (rafId) { window.cancelAnimationFrame(rafId); rafId = 0; }
    } else {
      ensure();
    }
  });

  return {
    add(fn) { subs.add(fn); ensure(); },
    remove(fn) {
      subs.delete(fn);
      if (subs.size === 0 && rafId) { window.cancelAnimationFrame(rafId); rafId = 0; }
    }
  };
})();

function setMotionState(on) {
  state.motionOn = Boolean(on);
  storageSet("singtail.motion", state.motionOn);
  document.documentElement.classList.toggle("motion-on", state.motionOn);
  const label = state.motionOn ? "(动效: 开)" : "(动效: 关)";
  ["motion-toggle-desktop", "motion-toggle-mobile"].forEach(id => {
    const btn = document.getElementById(id);
    if (btn) {
      btn.textContent = label;
      btn.setAttribute("aria-pressed", state.motionOn ? "true" : "false");
    }
  });
  window.dispatchEvent(new CustomEvent("singtail:motionchange", { detail: { on: state.motionOn } }));
}

const tipEl = document.getElementById("tip");
let tipTimer = 0;
function showTip(rect, text, immediate) {
  window.clearTimeout(tipTimer);
  if (!text) return;
  const run = () => {
    tipEl.textContent = text;
    tipEl.classList.add("is-visible");
    const vw = window.innerWidth, vh = window.innerHeight;
    const tw = tipEl.offsetWidth, th = tipEl.offsetHeight;
    let left = rect.right + 8;
    let top = rect.top - th - 8;
    if (left + tw > vw - 8) left = Math.max(8, rect.left - tw - 8);
    if (top < 8) top = Math.min(vh - th - 8, rect.bottom + 8);
    tipEl.style.left = `${Math.round(left)}px`;
    tipEl.style.top = `${Math.round(top)}px`;
  };
  if (immediate) run();
  else tipTimer = window.setTimeout(run, 100);
}
function hideTip() {
  window.clearTimeout(tipTimer);
  tipEl.classList.remove("is-visible");
}

const wipeCanvas = document.getElementById("wipe-canvas");
let wipeActive = false;
function playWipe(onFull) {
  if (!state.motionOn || wipeActive) { if (onFull) onFull(); return; }
  const ctx = wipeCanvas.getContext("2d");
  if (!ctx) { if (onFull) onFull(); return; }
  wipeActive = true;
  const w = window.innerWidth, h = window.innerHeight;
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  wipeCanvas.width = Math.round(w * dpr);
  wipeCanvas.height = Math.round(h * dpr);
  wipeCanvas.classList.add("is-active");
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

  const cell = w >= 768 ? 48 : 32;
  const cols = Math.ceil(w / cell), rows = Math.ceil(h / cell);
  const maxDiag = cols + rows - 2 || 1;
  const start = performance.now();
  let swapped = false;

  function step(now) {
    const elapsed = now - start;
    ctx.clearRect(0, 0, w, h);
    if (elapsed < 220) {
      const limit = (elapsed / 220) * (maxDiag + 1);
      for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
          if (c + r <= limit) {
            ctx.fillStyle = TRACK_COLORS[(c + r) % 4];
            ctx.fillRect(c * cell, r * cell, cell, cell);
          }
        }
      }
    } else {
      if (!swapped) { swapped = true; if (onFull) onFull(); }
      const prog = Math.min(1, (elapsed - 220) / 220);
      const limit = prog * (maxDiag + 1);
      for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
          if (c + r > limit) {
            ctx.fillStyle = TRACK_COLORS[(c + r) % 4];
            ctx.fillRect(c * cell, r * cell, cell, cell);
          }
        }
      }
      if (prog >= 1) {
        wipeCanvas.classList.remove("is-active");
        wipeActive = false;
        motion.remove(step);
      }
    }
  }
  motion.add(step);
}

function initTrail() {
  if (!window.matchMedia || !window.matchMedia("(pointer: fine)").matches) return;
  const trailCanvas = document.getElementById("trail-canvas");
  let trailCells = [], running = false, colorIdx = 0, lastKey = "";

  function size() {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    trailCanvas.width = Math.round(window.innerWidth * dpr);
    trailCanvas.height = Math.round(window.innerHeight * dpr);
    const ctx = trailCanvas.getContext("2d");
    if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
  size();
  window.addEventListener("resize", size, { passive: true });

  function tick(now) {
    const ctx = trailCanvas.getContext("2d");
    if (!ctx) return;
    ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);
    if (!state.motionOn) { trailCells = []; running = false; motion.remove(tick); return; }
    const alive = [];
    for (let i = 0; i < trailCells.length; i++) {
      const c = trailCells[i];
      const age = now - c.born;
      if (age >= 400) continue;
      alive.push(c);
      ctx.fillStyle = c.color;
      if (age < 130) ctx.fillRect(c.gx * 16, c.gy * 16, 16, 16);
      else if (age < 260) ctx.fillRect(c.gx * 16 + 4, c.gy * 16 + 4, 8, 8);
      else ctx.fillRect(c.gx * 16 + 6, c.gy * 16 + 6, 4, 4);
    }
    trailCells = alive;
    if (trailCells.length === 0) { running = false; motion.remove(tick); }
  }

  window.addEventListener("pointermove", e => {
    if (!state.motionOn || document.documentElement.classList.contains("menu-open")) return;
    const el = document.elementFromPoint(e.clientX, e.clientY);
    const surfEl = el && el.closest ? el.closest("[data-surface]") : null;
    if (surfEl && surfEl.getAttribute("data-surface") === "lime") return;
    const gx = Math.floor(e.clientX / 16), gy = Math.floor(e.clientY / 16);
    const key = `${gx},${gy}`;
    if (key === lastKey) return;
    lastKey = key;
    trailCells.push({ gx, gy, born: performance.now(), color: TRAIL_COLORS[colorIdx++ % TRAIL_COLORS.length] });
    if (trailCells.length > 14) trailCells.shift();
    if (!running) { running = true; motion.add(tick); }
  }, { passive: true });
}

function initChrome() {
  const logoLink = document.getElementById("header-logo-link");
  logoLink.innerHTML = brandIconSVG() + svgBitmapText(DATA.meta.name, 2.2, "currentColor", {
    perCell: true,
    className: "logo-svg",
    ariaHidden: true
  });

  if (window.matchMedia && window.matchMedia("(pointer: fine)").matches) {
    let jitterTimer = 0;
    logoLink.addEventListener("mouseenter", () => {
      if (!state.motionOn) return;
      const rects = Array.from(logoLink.querySelectorAll(".logo-svg rect"));
      const dirs = [[5, 0], [-5, 0], [0, 4], [0, -4], [5, 4], [-5, -4]];
      const rng = makeRng(Date.now());
      let step = 0;
      window.clearInterval(jitterTimer);
      const apply = () => {
        step++;
        if (step > 3) {
          rects.forEach(r => { r.setAttribute("x", String(Number(r.dataset.gx) * 5)); r.setAttribute("y", String(Number(r.dataset.gy) * 4)); });
          window.clearInterval(jitterTimer);
          return;
        }
        rects.forEach(r => {
          const bx = Number(r.dataset.gx) * 5, by = Number(r.dataset.gy) * 4;
          if (step === 3) { r.setAttribute("x", String(bx)); r.setAttribute("y", String(by)); }
          else { const d = dirs[Math.floor(rng() * dirs.length)]; r.setAttribute("x", String(bx + d[0])); r.setAttribute("y", String(by + d[1])); }
        });
      };
      apply();
      jitterTimer = window.setInterval(apply, 60);
    });
  }

  document.getElementById("header-nav").innerHTML = DATA.nav.map(n =>
    `<a class="nav-link t-micro u-anim" href="${esc(n.href)}">(${esc(n.label)})</a>`
  ).join("");

  document.getElementById("menu-nav").innerHTML = DATA.nav.map(n =>
    `<a class="t-h1" href="${esc(n.href)}">${esc(n.label)}</a>`
  ).join("");

  const footer = document.getElementById("site-footer");
  const stairCells = 96;
  let stairRects = "";
  for (let i = 0; i < stairCells; i++) {
    stairRects += `<rect x="${i}" y="0" width="1" height="1" fill="${TRACK_COLORS[Math.floor(i / 4) % 4]}"/>`;
  }
  footer.innerHTML = `
    <div class="grid">
      <div class="col-full">
        ${svgBitmapText(DATA.meta.name, 10, DATA.tokens.lime, { fluid: true, className: "footer-logo-svg", ariaHidden: true, bold: true })}
      </div>
    </div>
    <div class="grid footer-cols">
      <div class="footer-block">
        <p class="t-micro footer-block-head">(项目仓库)</p>
        <p class="t-small"><a class="u-anim" href="${esc(DATA.meta.repoUrl)}" target="_blank" rel="noopener noreferrer">github.com/evepupil/SingTail</a><br>Go + React 19 + Wails<br>Windows · macOS</p>
      </div>
      <div class="footer-block">
        <p class="t-micro footer-block-head">(内置核心)</p>
        <p class="t-small">${esc(DATA.meta.coreVersion)}<br>7 个基础 .srs 规则包<br>SQLite 多粒度流量账本</p>
      </div>
      <div class="footer-block">
        <p class="t-micro footer-block-head">(页面导航)</p>
        <ul class="footer-nav-list">
          <li><a class="btn-text t-small u-anim" href="#/">(首页)</a></li>
          ${DATA.nav.map(n => `<li><a class="btn-text t-small u-anim" href="${esc(n.href)}">(${esc(n.label)})</a></li>`).join("")}
        </ul>
      </div>
      <div class="footer-block">
        <p class="t-micro footer-block-head">(开源与隐私)</p>
        <p class="t-small">所有 Tailscale 组织私钥仅存于本机数据目录；导出配置自动脱敏，只保留分流规则与常规设置。</p>
      </div>
    </div>
    <div class="grid footer-bottom">
      <p class="footer-honesty t-small">SingTail — Multi-Org Tailscale Exit Gateway. 多组织接入、切出口验 IP 自动回滚、国内外智能分流与 Clash 无感共存。</p>
      <p class="footer-ver t-micro t-mono">${esc(DATA.meta.version)} · MIT License</p>
    </div>
    <svg class="footer-staircase" viewBox="0 0 ${stairCells} 1" preserveAspectRatio="none" shape-rendering="crispEdges" aria-hidden="true">${stairRects}</svg>
  `;

  document.getElementById("motion-toggle-desktop").addEventListener("click", () => setMotionState(!state.motionOn));
  document.getElementById("motion-toggle-mobile").addEventListener("click", () => setMotionState(!state.motionOn));
  setMotionState(state.motionOn);

  const menuEl = document.getElementById("menu");
  const openBtn = document.getElementById("menu-open-btn");
  const closeBtn = document.getElementById("menu-close-btn");
  const closeMenu = (returnFocus) => {
    menuEl.classList.remove("is-open");
    document.documentElement.classList.remove("menu-open");
    openBtn.setAttribute("aria-expanded", "false");
    if (returnFocus) openBtn.focus();
  };
  openBtn.addEventListener("click", () => playWipe(() => {
    menuEl.classList.add("is-open");
    document.documentElement.classList.add("menu-open");
    openBtn.setAttribute("aria-expanded", "true");
    closeBtn.focus();
  }));
  closeBtn.addEventListener("click", () => closeMenu(true));
  menuEl.addEventListener("click", e => { if (e.target.closest("a")) closeMenu(false); });
  document.addEventListener("keydown", e => {
    if (e.key === "Escape") {
      hideTip();
      if (menuEl.classList.contains("is-open")) closeMenu(true);
    }
  });

  document.getElementById("top-btn").addEventListener("click", () => {
    window.scrollTo({ top: 0, behavior: state.motionOn ? "smooth" : "auto" });
  });
}

let revealIO = null;
function setupObservers() {
  if (revealIO) revealIO.disconnect();
  const header = document.getElementById("site-header");
  const topBtn = document.getElementById("top-btn");
  const surfaceEls = Array.from(document.querySelectorAll("[data-surface]")).filter(
    el => el.id !== "site-header" && el.id !== "menu" && el.id !== "top-btn"
  );

  const syncSurfaces = () => {
    const hh = header.getBoundingClientRect().height || 64;
    const topY = hh + 4;
    const botY = window.innerHeight - 20;
    for (const el of surfaceEls) {
      const r = el.getBoundingClientRect();
      if (r.top <= topY && r.bottom >= topY) header.setAttribute("data-surface", el.getAttribute("data-surface"));
      if (r.top <= botY && r.bottom >= botY) topBtn.setAttribute("data-surface", el.getAttribute("data-surface"));
    }
    topBtn.classList.toggle("is-visible", window.scrollY >= window.innerHeight * 0.8);
  };
  window.addEventListener("scroll", syncSurfaces, { passive: true });
  state.routeCleanup.push(() => window.removeEventListener("scroll", syncSurfaces));
  syncSurfaces();

  const revealEls = Array.from(document.querySelectorAll(".reveal-text, .reveal-block"));
  if (!state.motionOn) {
    revealEls.forEach(el => { el.classList.remove("is-pending"); el.classList.add("is-shown"); });
  } else {
    revealEls.forEach(el => el.classList.add("is-pending"));
    revealIO = new IntersectionObserver(entries => {
      let batch = 0;
      for (const entry of entries) {
        if (entry.isIntersecting) {
          const el = entry.target;
          window.setTimeout(() => { el.classList.remove("is-pending"); el.classList.add("is-shown"); }, Math.min(batch++, 6) * 40);
          revealIO.unobserve(el);
        }
      }
    }, { threshold: 0.12 });
    revealEls.forEach(el => revealIO.observe(el));
  }
}

const runtime = { state, motion, showTip, hideTip };
let currentRouteKey = null;

function parseRoute(hash) {
  const clean = (hash || "#/").replace(/^#/, "");
  if (clean === "" || clean === "/") return { view: "home", key: "home" };
  const seg = clean.split("/").filter(Boolean)[0];
  if (["features", "architecture", "docs", "download", "sandbox"].includes(seg)) {
    return { view: seg, key: seg };
  }
  return { view: "404", key: "404" };
}

function updateActiveNav(route) {
  const href = route.view === "home" ? "" : `#/${route.view}`;
  document.querySelectorAll("#header-nav a, #menu-nav a").forEach(a => {
    if (a.getAttribute("href") === href) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  });
}

function mountRoute(route, skipWipe) {
  const main = document.getElementById("main");
  const doSwap = () => {
    while (state.routeCleanup.length > 0) {
      try { state.routeCleanup.pop()(); } catch {}
    }
    hideTip();
    if (route.view === "home") renderHome(main, runtime);
    else if (route.view === "features") renderFeatures(main, runtime);
    else if (route.view === "architecture") renderArchitecture(main);
    else if (route.view === "docs") renderDocs(main);
    else if (route.view === "download") renderDownload(main);
    else if (route.view === "sandbox") renderSandbox(main, runtime);
    else renderNotFound(main);

    updateActiveNav(route);
    setupObservers();
    window.scrollTo(0, 0);
  };
  if (skipWipe) doSwap();
  else playWipe(doSwap);
}

function init() {
  initChrome();
  initTrail();
  const initial = parseRoute(window.location.hash);
  currentRouteKey = initial.key;
  mountRoute(initial, true);

  window.addEventListener("hashchange", () => {
    const next = parseRoute(window.location.hash);
    if (next.key === currentRouteKey) return;
    currentRouteKey = next.key;
    mountRoute(next, false);
  });
}

document.addEventListener("DOMContentLoaded", init);
