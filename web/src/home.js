import { DATA } from "./data.js";
import {
  PATTERNS,
  TRACK_COLORS,
  buildModuleEmblemRows,
  copyText,
  easeAssemble,
  easeMorph,
  esc,
  hashStr,
  layoutGlyphLines,
  makeRng,
  svgBitmapText
} from "./graphics.js";

export function initEmblemCanvas(canvas, mod, runtime) {
  const rows = buildModuleEmblemRows(mod.id);
  const pal = mod.pal;
  let assembled = !runtime.state.motionOn;
  let assembleStart = 0;
  let waveStart = 0;
  let waveDir = 0;
  let swapped = false;
  let animActive = false;
  let lastSize = 0;

  const rng = makeRng(hashStr(mod.id));
  const fgIndices = [];
  for (let y = 0; y < 32; y++) {
    for (let x = 0; x < 32; x++) {
      if (rows[y][x] !== "0") fgIndices.push(y * 32 + x);
    }
  }
  for (let i = fgIndices.length - 1; i > 0; i--) {
    const j = Math.floor(rng() * (i + 1));
    const tmp = fgIndices[i];
    fgIndices[i] = fgIndices[j];
    fgIndices[j] = tmp;
  }
  const orderRank = new Int16Array(1024).fill(-1);
  fgIndices.forEach((pos, idx) => { orderRank[pos] = idx; });

  function sizeCanvas() {
    const rect = canvas.getBoundingClientRect();
    const cssSize = Math.round(rect.width);
    if (cssSize <= 0 || cssSize === lastSize) return cssSize > 0;
    lastSize = cssSize;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(cssSize * dpr);
    canvas.height = Math.round(cssSize * dpr);
    const ctx = canvas.getContext("2d");
    if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    return true;
  }

  function drawFrame(now) {
    const ctx = canvas.getContext("2d");
    if (!ctx || lastSize <= 0) return;
    const size = lastSize;
    const cell = Math.floor(size / 32);
    const gridPx = cell * 32;
    const ox = Math.floor((size - gridPx) / 2);
    const oy = Math.floor((size - gridPx) / 2);

    ctx.fillStyle = pal[0];
    ctx.fillRect(0, 0, size, size);

    let assembleLimit = fgIndices.length;
    if (!assembled) {
      const p = Math.min(1, Math.max(0, (now - assembleStart) / 560));
      assembleLimit = Math.floor(p * fgIndices.length);
      if (p >= 1) assembled = true;
    }

    let waveElapsed = -1;
    let waveDone = true;
    if (waveDir !== 0) {
      waveElapsed = now - waveStart;
      if (waveElapsed < 320) waveDone = false;
      else {
        swapped = waveDir === 1;
        waveDir = 0;
      }
    }

    for (let y = 0; y < 32; y++) {
      let runStart = 0;
      let runIdx = -1;
      for (let x = 0; x <= 32; x++) {
        let idx = 0;
        if (x < 32) {
          const raw = Number(rows[y][x]) || 0;
          if (!assembled && raw !== 0) {
            idx = orderRank[y * 32 + x] < assembleLimit ? raw : 0;
          } else {
            idx = raw;
          }
          let isSwapped = swapped;
          if (waveDir !== 0) {
            const threshold = ((x + y) / 62) * 320;
            isSwapped = waveDir === 1 ? waveElapsed >= threshold : waveElapsed < threshold;
          }
          if (isSwapped && (idx === 0 || idx === 1)) {
            idx = idx === 0 ? 1 : 0;
          }
        }
        if (x === 0) {
          runIdx = idx;
          runStart = 0;
        } else if (x === 32 || idx !== runIdx) {
          if (runIdx !== 0 || swapped || waveDir !== 0) {
            ctx.fillStyle = pal[runIdx];
            ctx.fillRect(ox + runStart * cell, oy + y * cell, (x - runStart) * cell, cell);
          }
          runStart = x;
          runIdx = idx;
        }
      }
    }

    if (assembled && waveDone) {
      animActive = false;
      runtime.motion.remove(drawFrame);
    }
  }

  function startAnim() {
    if (!animActive) {
      animActive = true;
      runtime.motion.add(drawFrame);
    }
  }

  const io = new IntersectionObserver(entries => {
    for (const entry of entries) {
      if (entry.isIntersecting) {
        io.disconnect();
        sizeCanvas();
        if (!runtime.state.motionOn) {
          assembled = true;
          drawFrame(performance.now());
        } else {
          assembled = false;
          assembleStart = performance.now();
          startAnim();
        }
      }
    }
  }, { threshold: 0.25 });
  io.observe(canvas);

  let rTimer = 0;
  const ro = new ResizeObserver(() => {
    window.clearTimeout(rTimer);
    rTimer = window.setTimeout(() => {
      const prev = lastSize;
      sizeCanvas();
      if (lastSize !== prev) drawFrame(performance.now());
    }, 150);
  });
  ro.observe(canvas);

  const hoverTarget = canvas.closest("a") || canvas;
  if (window.matchMedia && window.matchMedia("(hover: hover)").matches) {
    const triggerSwap = toSwapped => {
      if (!runtime.state.motionOn) {
        swapped = toSwapped;
        waveDir = 0;
        drawFrame(performance.now());
        return;
      }
      waveDir = toSwapped ? 1 : -1;
      waveStart = performance.now();
      startAnim();
    };
    hoverTarget.addEventListener("mouseenter", () => triggerSwap(true));
    hoverTarget.addEventListener("mouseleave", () => triggerSwap(false));
    hoverTarget.addEventListener("focusin", () => triggerSwap(true));
    hoverTarget.addEventListener("focusout", () => triggerSwap(false));
  }

  const onMotionChange = () => {
    if (!runtime.state.motionOn) {
      assembled = true;
      waveDir = 0;
      swapped = false;
      if (sizeCanvas()) drawFrame(performance.now());
    }
  };
  window.addEventListener("singtail:motionchange", onMotionChange);

  runtime.state.routeCleanup.push(() => {
    io.disconnect();
    ro.disconnect();
    window.clearTimeout(rTimer);
    runtime.motion.remove(drawFrame);
    window.removeEventListener("singtail:motionchange", onMotionChange);
  });

  window.requestAnimationFrame(() => {
    if (sizeCanvas()) drawFrame(performance.now());
  });
}

function initHeroCanvas(runtime) {
  const canvas = document.getElementById("hero-canvas");
  const captionEl = document.getElementById("hero-cycle-caption");
  const ctx = canvas ? canvas.getContext("2d") : null;
  if (!ctx) return;

  let cssW = 0, cssH = 0, dpr = 1;
  let isMobile = window.innerWidth < 768;
  let cycleIdx = 0;
  let particles = [];
  let morphStart = 0;
  let morphDuration = 1100;
  let isAssembling = true;
  let holdTimer = 0;
  let inView = true;
  let running = false;
  let pointerX = -9999, pointerY = -9999;

  function getTargetLayout(idx) {
    const item = DATA.copy.heroCycle[idx];
    const lines = isMobile ? item.mobile : [item.desktop];
    const layout = layoutGlyphLines(lines, 2, true);
    const totalCols = isMobile ? Math.max(22, layout.cols + 4) : Math.max(48, layout.cols + 4);
    const totalRows = isMobile ? layout.rows + 2 : 11;
    const offsetX = Math.floor((totalCols - layout.cols) / 2);
    const offsetY = isMobile ? 1 : 1;
    return {
      totalCols,
      totalRows,
      cells: layout.cells.map(c => ({ x: c.x + offsetX, y: c.y + offsetY }))
    };
  }

  function sizeCanvas() {
    const rect = canvas.getBoundingClientRect();
    isMobile = window.innerWidth < 768;
    cssW = rect.width;
    cssH = rect.height;
    if (cssW <= 0 || cssH <= 0) return;
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(cssW * dpr);
    canvas.height = Math.round(cssH * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  function buildParticlesForTarget(idx, assembleFromRandom) {
    const t = getTargetLayout(idx);
    const rng = makeRng(hashStr("singtail-hero-" + idx + "-" + Date.now()));
    const next = [];
    const targetCells = t.cells;
    const oldParticles = particles;

    if (assembleFromRandom || oldParticles.length === 0) {
      for (let i = 0; i < targetCells.length; i++) {
        const tc = targetCells[i];
        next.push({
          sx: assembleFromRandom ? rng() * t.totalCols : tc.x,
          sy: assembleFromRandom ? rng() * t.totalRows : tc.y,
          tx: tc.x, ty: tc.y, x: tc.x, y: tc.y, vx: 0, vy: 0,
          delay: assembleFromRandom ? rng() * 380 : 0,
          color: DATA.tokens.lime, sparkle: false
        });
      }
      isAssembling = assembleFromRandom;
      morphDuration = 1100;
    } else {
      const count = Math.max(oldParticles.length, targetCells.length);
      for (let i = 0; i < count; i++) {
        const src = oldParticles[i % oldParticles.length];
        const dst = targetCells[i % targetCells.length];
        const isSparkle = rng() < 0.16;
        next.push({
          sx: src.x, sy: src.y, tx: dst.x, ty: dst.y, x: src.x, y: src.y, vx: 0, vy: 0,
          delay: (dst.x / t.totalCols) * 280,
          color: isSparkle ? TRACK_COLORS[Math.floor(rng() * 4)] : DATA.tokens.lime,
          sparkle: isSparkle
        });
      }
      isAssembling = false;
      morphDuration = 850;
    }
    particles = next;
    morphStart = performance.now();
    captionEl.textContent = DATA.copy.heroCycle[idx].caption;
  }

  function drawBackdrop(cellW, cellH, totalCols, totalRows, oy) {
    ctx.fillStyle = DATA.tokens.rail;
    ctx.fillRect(0, 0, cssW, cssH);
    // Subtle 1px architectural telemetry grid lines inside hero display
    ctx.strokeStyle = "rgba(255, 255, 255, 0.04)";
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (let c = 0; c <= totalCols; c += 4) {
      const x = Math.round(c * cellW) + 0.5;
      ctx.moveTo(x, 0);
      ctx.lineTo(x, cssH);
    }
    for (let r = 0; r <= totalRows; r += 2) {
      const y = Math.round(oy + r * cellH) + 0.5;
      ctx.moveTo(0, y);
      ctx.lineTo(cssW, y);
    }
    ctx.stroke();
  }

  function drawStatic() {
    sizeCanvas();
    if (cssW <= 0) return;
    const t = getTargetLayout(0);
    const cellW = cssW / t.totalCols;
    const cellH = Math.min(cellW / 1.15, cssH / t.totalRows);
    const oy = (cssH - cellH * t.totalRows) / 2;
    drawBackdrop(cellW, cellH, t.totalCols, t.totalRows, oy);
    ctx.fillStyle = DATA.tokens.lime;
    for (let i = 0; i < t.cells.length; i++) {
      const c = t.cells[i];
      ctx.fillRect(Math.floor(c.x * cellW), Math.floor(oy + c.y * cellH), Math.ceil(cellW), Math.ceil(cellH));
    }
    captionEl.textContent = DATA.copy.heroCycle[0].caption;
  }

  function step(now) {
    if (!runtime.state.motionOn) {
      running = false;
      runtime.motion.remove(step);
      drawStatic();
      return;
    }
    if (cssW <= 0) sizeCanvas();
    if (cssW <= 0) return;

    const t = getTargetLayout(cycleIdx);
    const cellW = cssW / t.totalCols;
    const cellH = Math.min(cellW / 1.15, cssH / t.totalRows);
    const oy = (cssH - cellH * t.totalRows) / 2;
    const elapsed = now - morphStart;
    const easeFn = isAssembling ? easeAssemble : easeMorph;
    const repelRadius = cellW * 3;
    const maxDisp = 1.4;

    drawBackdrop(cellW, cellH, t.totalCols, t.totalRows, oy);

    const buckets = new Map();
    buckets.set(DATA.tokens.lime, []);

    for (let i = 0; i < particles.length; i++) {
      const p = particles[i];
      const localT = Math.min(1, Math.max(0, (elapsed - p.delay) / morphDuration));
      const e = easeFn(localT);
      const baseX = p.sx + (p.tx - p.sx) * e;
      const baseY = p.sy + (p.ty - p.sy) * e;

      let targetDx = 0, targetDy = 0;
      if (pointerX > -5000) {
        const px = (baseX + 0.5) * cellW;
        const py = oy + (baseY + 0.5) * cellH;
        const dx = px - pointerX;
        const dy = py - pointerY;
        const dist = Math.hypot(dx, dy);
        if (dist > 0.001 && dist < repelRadius) {
          const push = (1 - dist / repelRadius) * maxDisp;
          targetDx = (dx / dist) * push;
          targetDy = (dy / dist) * push;
        }
      }
      if (localT >= 1 && pointerX < -5000) {
        p.vx = 0; p.vy = 0; p.x = p.tx; p.y = p.ty;
      } else {
        p.vx = (p.vx + (targetDx - (p.x - baseX)) * 0.12) * 0.8;
        p.vy = (p.vy + (targetDy - (p.y - baseY)) * 0.12) * 0.8;
        p.x = baseX + (p.x - baseX) + p.vx;
        p.y = baseY + (p.y - baseY) + p.vy;
      }

      const col = (p.sparkle && elapsed < 420) ? p.color : DATA.tokens.lime;
      if (!buckets.has(col)) buckets.set(col, []);
      buckets.get(col).push(p);
    }

    buckets.forEach((list, col) => {
      ctx.fillStyle = col;
      for (let i = 0; i < list.length; i++) {
        const p = list[i];
        const x0 = Math.round(p.x * cellW * dpr) / dpr;
        const y0 = Math.round((oy + p.y * cellH) * dpr) / dpr;
        const x1 = Math.round((p.x + 1) * cellW * dpr) / dpr;
        const y1 = Math.round((oy + (p.y + 1) * cellH) * dpr) / dpr;
        ctx.fillRect(x0, y0, x1 - x0 + 0.3, y1 - y0 + 0.3);
      }
    });
  }

  function scheduleNext() {
    window.clearTimeout(holdTimer);
    if (!runtime.state.motionOn || !inView) return;
    holdTimer = window.setTimeout(() => {
      if (!runtime.state.motionOn || !inView || document.hidden) return;
      cycleIdx = (cycleIdx + 1) % DATA.copy.heroCycle.length;
      buildParticlesForTarget(cycleIdx, false);
      scheduleNext();
    }, 3400 + morphDuration);
  }

  function startLoop() {
    if (!runtime.state.motionOn) { drawStatic(); return; }
    if (!running) { running = true; runtime.motion.add(step); }
    scheduleNext();
  }

  function stopLoop() {
    window.clearTimeout(holdTimer);
    if (running) { running = false; runtime.motion.remove(step); }
  }

  sizeCanvas();
  if (runtime.state.motionOn) { buildParticlesForTarget(0, true); startLoop(); }
  else { drawStatic(); }

  canvas.addEventListener("click", () => {
    if (!runtime.state.motionOn) return;
    cycleIdx = (cycleIdx + 1) % DATA.copy.heroCycle.length;
    buildParticlesForTarget(cycleIdx, false);
    scheduleNext();
  });

  if (window.matchMedia && window.matchMedia("(pointer: fine)").matches) {
    canvas.addEventListener("pointermove", e => {
      const r = canvas.getBoundingClientRect();
      pointerX = e.clientX - r.left;
      pointerY = e.clientY - r.top;
    }, { passive: true });
    canvas.addEventListener("pointerleave", () => { pointerX = -9999; pointerY = -9999; });
  }

  const io = new IntersectionObserver(entries => {
    for (const entry of entries) {
      inView = entry.intersectionRatio >= 0.1;
      if (inView && runtime.state.motionOn) startLoop();
      else stopLoop();
    }
  }, { threshold: [0, 0.1] });
  io.observe(canvas);

  let rTimer = 0;
  const ro = new ResizeObserver(() => {
    window.clearTimeout(rTimer);
    rTimer = window.setTimeout(() => {
      const wasMob = isMobile;
      sizeCanvas();
      if (!runtime.state.motionOn) drawStatic();
      else if (wasMob !== isMobile) buildParticlesForTarget(cycleIdx, false);
    }, 150);
  });
  ro.observe(canvas);

  const onMotionChange = () => {
    if (runtime.state.motionOn) { cycleIdx = 0; buildParticlesForTarget(0, true); startLoop(); }
    else { stopLoop(); drawStatic(); }
  };
  window.addEventListener("singtail:motionchange", onMotionChange);

  runtime.state.routeCleanup.push(() => {
    stopLoop(); io.disconnect(); ro.disconnect(); window.clearTimeout(rTimer);
    window.removeEventListener("singtail:motionchange", onMotionChange);
  });
}

function initChassisGrid(container, runtime) {
  const total = 200;
  const isMobile = window.innerWidth < 768;
  const cols = isMobile ? 20 : 40;
  const rows = Math.ceil(total / cols);
  const gap = window.innerWidth >= 1024 ? 2 : 1;
  const cell = 10;
  const step = cell + gap;
  const vbW = cols * step - gap;
  const vbH = rows * step - gap;

  const t0 = DATA.ruleChassis[0];
  const t1 = DATA.ruleChassis[1];
  const t2 = DATA.ruleChassis[2];
  const t3 = DATA.ruleChassis[3];
  const usedTotal = t0.count + t1.count + t2.count + t3.count;
  const openTotal = total - usedTotal;

  const cellsMeta = [];
  for (let i = 0; i < total; i++) {
    let tier = "open", color = DATA.tokens.lime, label = `单元 ${i + 1} / 200 · 可扩展个人规则槽位`;
    if (i < t0.count) {
      tier = "cn"; color = t0.color; label = `单元 ${i + 1} / 200 · geosite-cn 国内直连规则`;
    } else if (i < t0.count + t1.count) {
      tier = "ip"; color = t1.color; label = `单元 ${i + 1} / 200 · geoip-cn + 8.128.0.0/10 修正段`;
    } else if (i < t0.count + t1.count + t2.count) {
      tier = "exit"; color = t2.color; label = `单元 ${i + 1} / 200 · 境外 / AI 规则集走选定出口`;
    } else if (i < usedTotal) {
      tier = "proc"; color = t3.color; label = `单元 ${i + 1} / 200 · Clash 放行与进程点选规则`;
    }
    cellsMeta.push({ idx: i, tier, color, label });
  }

  let rects = "";
  for (let i = 0; i < total; i++) {
    const c = i % cols;
    const r = Math.floor(i / cols);
    const x = c * step;
    const y = r * step;
    rects += `<rect data-ci="${i}" x="${x + 0.5}" y="${y + 0.5}" width="${cell - 1}" height="${cell - 1}" fill="${DATA.tokens.lime}" stroke="${DATA.tokens.ink}" stroke-width="1"/>`;
  }

  container.innerHTML = `
    <figure class="capacity-fig">
      <div class="capacity-svg-wrap" tabindex="0" role="img" aria-label="SingTail 内置分流底盘矩阵：共 200 单元，已预置 ${usedTotal} 单元默认分流规则，保留 ${openTotal} 单元供用户自定义。可用方向键逐格查看。">
        <svg class="capacity-svg" viewBox="0 0 ${vbW} ${vbH}" shape-rendering="crispEdges" aria-hidden="true">${rects}<rect id="cap-focus-ring" x="0" y="0" width="${cell}" height="${cell}" fill="none" stroke="${DATA.tokens.ink}" stroke-width="2" visibility="hidden"/></svg>
      </div>
      <figcaption class="t-micro" style="margin-top:8px;">(Fig. 1 — SingTail 开箱即用分流底盘：内置 7 个 .srs 规则包与防冲突进程放行，并预留自定义规则与订阅扩展位。)</figcaption>
      <ul class="capacity-legend t-micro">
        ${DATA.ruleChassis.map(t => `<li class="legend-item"><span class="legend-swatch" style="background:${t.color}"></span>${esc(t.type)} (${esc(t.scope)})</li>`).join("")}
        <li class="legend-item"><span class="legend-swatch" style="background:${DATA.tokens.rail}"></span>用户自定义扩展位 · ${openTotal} 格</li>
      </ul>
    </figure>
  `;

  const svg = container.querySelector(".capacity-svg");
  const wrap = container.querySelector(".capacity-svg-wrap");
  const focusRing = svg.querySelector("#cap-focus-ring");
  const rectEls = Array.from(svg.querySelectorAll("rect[data-ci]"));
  let filledCount = 0;
  let fillAnimRunning = false;

  function paintCell(i) {
    const m = cellsMeta[i];
    const el = rectEls[i];
    if (!el) return;
    if (m.tier === "open" || i >= filledCount) {
      el.setAttribute("fill", i >= filledCount ? DATA.tokens.lime : DATA.tokens.rail);
      el.setAttribute("stroke", DATA.tokens.ink);
    } else {
      el.setAttribute("fill", m.color);
      el.setAttribute("stroke", DATA.tokens.ink);
    }
  }

  function fillImmediate() {
    filledCount = usedTotal;
    for (let i = 0; i < total; i++) paintCell(i);
  }

  function tickFill() {
    const nextEnd = Math.min(usedTotal, filledCount + 4);
    for (let i = filledCount; i < nextEnd; i++) {
      filledCount = i + 1;
      paintCell(i);
    }
    if (filledCount >= usedTotal) {
      fillAnimRunning = false;
      runtime.motion.remove(tickFill);
      for (let i = usedTotal; i < total; i++) paintCell(i);
    }
  }

  if (!runtime.state.motionOn) {
    fillImmediate();
  } else {
    const io = new IntersectionObserver(entries => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          io.disconnect();
          if (!runtime.state.motionOn) fillImmediate();
          else if (!fillAnimRunning) { fillAnimRunning = true; runtime.motion.add(tickFill); }
        }
      }
    }, { threshold: 0.3 });
    io.observe(svg);
    runtime.state.routeCleanup.push(() => { io.disconnect(); runtime.motion.remove(tickFill); });
  }

  svg.addEventListener("pointermove", e => {
    const rectEl = e.target.closest("rect[data-ci]");
    if (!rectEl) { runtime.hideTip(); return; }
    const idx = Number(rectEl.dataset.ci);
    runtime.showTip(rectEl.getBoundingClientRect(), cellsMeta[idx].label, false);
  });
  svg.addEventListener("pointerleave", runtime.hideTip);

  let keyIdx = 0;
  const showKeyCell = () => {
    const c = keyIdx % cols;
    const r = Math.floor(keyIdx / cols);
    focusRing.setAttribute("x", String(c * step - 1));
    focusRing.setAttribute("y", String(r * step - 1));
    focusRing.setAttribute("width", String(cell + 2));
    focusRing.setAttribute("height", String(cell + 2));
    focusRing.setAttribute("visibility", "visible");
    runtime.showTip(rectEls[keyIdx].getBoundingClientRect(), cellsMeta[keyIdx].label, true);
  };
  wrap.addEventListener("focus", showKeyCell);
  wrap.addEventListener("blur", () => { focusRing.setAttribute("visibility", "hidden"); runtime.hideTip(); });
  wrap.addEventListener("keydown", e => {
    if (e.key === "ArrowRight") keyIdx = Math.min(total - 1, keyIdx + 1);
    else if (e.key === "ArrowLeft") keyIdx = Math.max(0, keyIdx - 1);
    else if (e.key === "ArrowDown") keyIdx = Math.min(total - 1, keyIdx + cols);
    else if (e.key === "ArrowUp") keyIdx = Math.max(0, keyIdx - cols);
    else return;
    e.preventDefault();
    showKeyCell();
  });
}

export function renderHome(main, runtime) {
  document.title = DATA.copy.homeTitle;
  const c = DATA.copy;
  const quickCmd = "irm https://github.com/evepupil/SingTail/releases/latest | iex";
  const releaseSummary = "Windows Setup / Portable · macOS arm64 & amd64 DMG";

  main.innerHTML = `
    <section class="hero-sec" id="hero" data-surface="lime">
      <h1 class="visually-hidden">${esc(c.heroH1)}</h1>
      <div class="grid" style="padding-inline:0;">
        <div class="hero-top-meta t-lead reveal-text">
          ${c.heroMeta.map(l => `<div>${esc(l)}</div>`).join("")}
        </div>
        <div class="hero-top-date t-micro tnum">
          <a class="hero-repo-link" href="${esc(DATA.meta.repoUrl)}" target="_blank" rel="noopener noreferrer" aria-label="前往 GitHub 仓库 evepupil/SingTail">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 1.27a11 11 0 0 0-3.48 21.46c.55.09.73-.28.73-.55v-1.84c-3.03.64-3.67-1.46-3.67-1.46-.55-1.29-1.28-1.65-1.28-1.65-.92-.65.1-.65.1-.65 1.1 0 1.73 1.1 1.73 1.1.92 1.65 2.57 1.2 3.21.92a2 2 0 0 1 .64-1.47c-2.47-.27-5.04-1.19-5.04-5.5 0-1.1.46-2.1 1.2-2.84a3.76 3.76 0 0 1 0-2.93s.91-.28 3.11 1.1c1.8-.49 3.7-.49 5.5 0 2.1-1.38 3.02-1.1 3.02-1.1a3.76 3.76 0 0 1 0 2.93c.83.74 1.2 1.74 1.2 2.94 0 4.21-2.57 5.13-5.04 5.4.45.37.82.92.82 2.02v3.03c0 .27.1.64.73.55A11 11 0 0 0 12 1.27Z"/></svg>
            <span>evepupil/SingTail</span>
            <span style="opacity:0.78;">· ${esc(DATA.meta.version)} · MIT ↗</span>
          </a>
        </div>
      </div>

      <figure class="hero-display-wrap">
        <canvas class="hero-canvas" id="hero-canvas" aria-hidden="true"></canvas>
        <div class="grid hero-sub-row" style="padding-inline:0;">
          <figcaption class="hero-caption t-micro" id="hero-cycle-caption">${esc(c.heroCycle[0].caption)}</figcaption>
          <div class="hero-cmd-strip">
            <div class="cmd-inline">
              <span>${esc(releaseSummary)}</span>
              <button type="button" class="cmd-copy-btn" id="hero-copy-cmd">复制源仓库地址</button>
            </div>
          </div>
        </div>
      </figure>

      <div class="grid hero-bottom-row" style="padding-inline:0;">
        <div class="hero-cta-col">
          <a class="btn-primary t-small" href="#/download"><span>获取安装包 (${esc(DATA.meta.version)})</span></a>
          <a class="btn-outline t-small" href="${esc(DATA.meta.repoUrl)}" target="_blank" rel="noopener noreferrer">GitHub 源码</a>
        </div>
        <div class="hero-split-wrap t-lead reveal-text">
          ${c.heroSplit.map(line => {
            const onlyR = !line.left && line.right ? " only-right" : "";
            return `<div class="split-line${onlyR}"><span>${esc(line.left)}</span><span class="frag-right">${esc(line.right)}</span></div>`;
          }).join("")}
        </div>
      </div>
    </section>

    <section class="sec" id="about" data-surface="sand">
      <div class="grid">
        <p class="col-1-3 t-micro">(为什么做 SingTail)</p>
        <h2 class="col-5-12 t-h2 reveal-text">${esc(c.aboutLead)}</h2>
      </div>
      <div class="grid about-paras">
        <p class="about-para-1 t-body reveal-text">${esc(c.aboutP1)}</p>
        <p class="about-para-2 t-body reveal-text">${esc(c.aboutP2)}</p>
      </div>
      <div class="grid">
        <div class="col-full pain-list">
          ${DATA.pains.map(p => `
            <div class="pain-row">
              <div class="t-small fw-600">${esc(p.label)}</div>
              <div class="t-small">${esc(p.problem)}</div>
              <div class="t-small fw-600">${esc(p.solution)}</div>
            </div>
          `).join("")}
        </div>
      </div>
    </section>

    <section class="sec" id="numbers" data-surface="charcoal">
      <h2 class="visually-hidden">核心参数与出口切换演练</h2>
      <div class="grid">
        <p class="col-1-3 t-micro t-lime">(硬核参数)</p>
        <p class="col-5-12 t-small t-paper">纯 Go 零 CGO 交叉编译桌面壳与后台服务，内置 sing-box 1.14.3 与 7 个基础 .srs 规则包。</p>
      </div>
      <div class="grid stats-grid-row">
        <div class="stats-items" id="stats-items"></div>
      </div>
      <div class="grid relay-block">
        <div class="col-full">
          <p class="t-micro t-lime">(多组织出口切换与验 IP 回滚演练 · 点击切换阶段)</p>
          <div class="relay-controls" id="relay-controls">
            ${DATA.relayScenarios.map((sc, i) => `
              <button type="button" class="chip-toggle t-small" data-relay="${esc(sc.id)}" aria-pressed="${i === 0 ? "true" : "false"}">${esc(sc.step)} · ${esc(sc.client)}</button>
            `).join("")}
          </div>
          <div class="relay-stage" id="relay-stage"></div>
        </div>
      </div>
    </section>

    <section class="sec" id="pillars" data-surface="charcoal">
      <div class="grid">
        <p class="col-1-3 t-micro t-lime">(核心优势)</p>
        <h2 class="col-5-12 t-h2 t-paper reveal-text">${esc(c.pillarsLead)}</h2>
      </div>
      <div class="grid tracks-grid" id="tracks-grid"></div>
    </section>

    <section class="sec" id="workflow" data-surface="sand">
      <div class="grid">
        <p class="col-1-3 t-micro">(上手流程)</p>
        <h2 class="col-5-12 t-h2 reveal-text">${esc(c.workflowLead)}</h2>
      </div>
      <div class="grid days-cols" id="days-cols"></div>
    </section>

    <section class="sec" id="modules-preview" data-surface="lime">
      <div class="grid">
        <p class="col-1-3 t-micro">(内置能力矩阵)</p>
        <h2 class="col-5-12 t-h2 reveal-text">${esc(c.clientsLead)}</h2>
        <div class="col-full speakers-preview-grid" id="modules-preview-grid"></div>
        <div class="col-full speakers-footer-link">
          <a class="btn-text t-small u-anim" href="#/features">(查看全部 8 大模块与底层包实现详情 →)</a>
        </div>
      </div>
    </section>

    <section class="sec" id="poster" data-surface="sand">
      <div class="grid">
        <div class="poster-head-left t-small">
          ${c.posterLeftHead.map(l => `<div>${esc(l)}</div>`).join("")}
        </div>
        <h2 class="poster-head-right t-h2">
          ${c.posterRightHead.map(l => `<div>${esc(l)}</div>`).join("")}
        </h2>
      </div>
      <div class="grid">
        <figure class="col-full">
          <div class="poster-wall-wrap" id="poster-wall"></div>
          <figcaption class="poster-caption t-micro">(点击任意工程纹样方块切换配色。四套全新纹样分别映射：节点拓扑矩阵、实时流量波形、建筑准星与加密隧道跃点。)</figcaption>
        </figure>
      </div>
    </section>

    <section class="sec" id="chassis" data-surface="lime">
      <div class="grid">
        <p class="col-1-3 t-micro">(分流底盘与扩展性)</p>
        <h2 class="col-5-12 t-h2 reveal-text">开箱即用的国内外防污染分流底盘，同时支持规则导入与订阅更新。</h2>
        <div class="col-full" id="home-chassis-wrap"></div>
        <div class="col-full passes-cta-row">
          <a class="btn-primary t-small" href="#/download"><span>选择我的系统安装包</span></a>
          <a class="btn-outline t-small" href="#/sandbox">在线测试规则解析沙箱</a>
          <a class="btn-text t-small u-anim" href="#/docs">(查阅守护接口与安装包清单)</a>
        </div>
      </div>
    </section>
  `;

  document.getElementById("hero-copy-cmd").addEventListener("click", e => {
    copyText(DATA.meta.repoUrl, e.currentTarget);
  });

  initHeroCanvas(runtime);

  const statsWrap = document.getElementById("stats-items");
  statsWrap.innerHTML = DATA.stats.map((s, idx) => `
    <div class="stat-item" data-sidx="${idx}">
      <div class="stat-glyph">${svgBitmapText(s.val, 10, DATA.tokens.lime, { className: "stat-svg", ariaHidden: true, bold: true })}</div>
      <p class="t-small t-lime stat-label fw-600">${esc(s.label)}</p>
      <p class="t-micro t-paper" style="opacity:0.75;margin-top:4px;">${esc(s.sub)}</p>
    </div>
  `).join("");

  // Exit Switch & Rollback Simulator
  const relayStage = document.getElementById("relay-stage");
  const relayBtns = Array.from(document.querySelectorAll("#relay-controls [data-relay]"));
  function drawRelay(id) {
    const sc = DATA.relayScenarios.find(x => x.id === id) || DATA.relayScenarios[0];
    relayBtns.forEach(b => b.setAttribute("aria-pressed", b.dataset.relay === sc.id ? "true" : "false"));
    relayStage.innerHTML = `
      <div>
        <p class="t-micro t-lime">阶段 ${esc(sc.step)} · ${esc(sc.client)} (${esc(sc.channel)})</p>
        <p class="t-lead t-paper" style="margin-top:12px;">${esc(sc.prompt)}</p>
        <p class="t-small t-paper" style="margin-top:16px;opacity:0.86;">${esc(sc.action)}</p>
        <p class="t-micro t-mono t-cyan" style="margin-top:16px;">路由与状态路径: ${esc(sc.file)}</p>
      </div>
      <pre class="relay-code">${esc(sc.code)}</pre>
    `;
  }
  relayBtns.forEach(b => b.addEventListener("click", () => drawRelay(b.dataset.relay)));
  drawRelay(DATA.relayScenarios[0].id);

  // 4 Pillar Pattern Panels (using SingTail's 4 new patterns)
  const tracksGrid = document.getElementById("tracks-grid");
  const patternKeys = ["nodeMatrix", "tunnelHop", "crosshairGrid", "telemetryWave"];
  tracksGrid.innerHTML = DATA.pillars.map(p => `
    <div class="track-fig">
      <button type="button" class="track-panel-btn" data-pillar="${esc(p.id)}" data-route="${esc(p.route)}" style="--track-band:${p.color};--track-ink:${p.fg};color:${p.fg};">
        <svg class="track-panel-svg" viewBox="0 0 80 68" shape-rendering="crispEdges" aria-hidden="true"></svg>
        <div class="track-card-body">
          <div>
            <p class="t-micro" style="opacity:0.78;">${esc(p.en)}</p>
            <p class="track-card-title" style="margin-top:6px;">${esc(p.label)}</p>
            <p class="t-small track-card-desc">${esc(p.desc)}</p>
          </div>
          <div class="track-card-foot">
            <span class="t-micro fw-600">${esc(p.metric)}</span>
            <svg class="track-card-arrow" width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M2 12L12 2M12 2H4M12 2V10" stroke="currentColor" stroke-width="1.6" stroke-linecap="square"/>
            </svg>
          </div>
        </div>
      </button>
    </div>
  `).join("");

  DATA.pillars.forEach((p, idx) => {
    const btn = tracksGrid.querySelector(`[data-pillar="${p.id}"]`);
    const svg = btn.querySelector(".track-panel-svg");
    const pat = PATTERNS[patternKeys[idx % 4]];
    let timer = 0, busy = false;
    function drawGrid(phase) {
      let out = `<rect x="0" y="0" width="80" height="68" fill="${DATA.tokens.rail}"/>`;
      for (let y = 0; y < 17; y++) {
        for (let x = 0; x < 20; x++) {
          if (pat(x, y, phase) === 1) {
            out += `<rect x="${x * 4}" y="${y * 4}" width="4" height="4" fill="${p.color}"/>`;
          }
        }
      }
      svg.innerHTML = out;
    }
    drawGrid(0);
    const play = () => {
      if (!runtime.state.motionOn || busy) return;
      busy = true;
      let step = 0;
      timer = window.setInterval(() => {
        step++;
        drawGrid(step % 8);
        if (step >= 6) { window.clearInterval(timer); drawGrid(0); busy = false; }
      }, 80);
    };
    btn.addEventListener("mouseenter", play);
    btn.addEventListener("click", () => { window.location.hash = p.route; });
    runtime.state.routeCleanup.push(() => window.clearInterval(timer));
  });

  // 3-Step Workflow
  const daysCols = document.getElementById("days-cols");
  const tagClassMap = { "multi-org": "tag-lime", routing: "tag-cyan", coexist: "tag-green", stats: "tag-amber" };
  daysCols.innerHTML = DATA.workflowSteps.map(step => `
    <div class="day-col">
      ${svgBitmapText(step.num, 8, DATA.tokens.ink, { className: "day-num-svg", ariaHidden: true, bold: true })}
      <h3 class="t-lead">${esc(step.title)}</h3>
      <p class="t-micro t-mono day-doors">${esc(step.sub)}</p>
      <div class="day-session-list">
        ${step.items.map(it => `
          <a class="day-session-row" href="#/docs" data-cat="${esc(it.cat)}">
            <div class="t-micro t-mono">${esc(it.time)}</div>
            <div class="t-small day-session-title">${esc(it.title)}</div>
            <p class="t-micro" style="margin-top:6px;line-height:1.5;">${esc(it.desc)}</p>
            <div class="day-session-meta">
              <span class="tag-inline t-micro ${tagClassMap[it.cat] || "tag-lime"}">${esc(it.tag)}</span>
            </div>
          </a>
        `).join("")}
      </div>
    </div>
  `).join("");

  // 8 Modules Pixel Matrix
  const modGrid = document.getElementById("modules-preview-grid");
  modGrid.innerHTML = DATA.modules.map(m => `
    <figure class="speaker-card">
      <a class="speaker-card-link" href="#/features">
        <canvas class="portrait-canvas" data-modid="${esc(m.id)}" role="img" aria-label="${esc(m.name)} 像素模块图标"></canvas>
        <figcaption class="speaker-card-figcap">
          <h3 class="t-lead speaker-card-name">${esc(m.name)}</h3>
          <p class="t-micro speaker-card-role">${esc(m.mode)}</p>
          <p class="t-micro t-mono speaker-card-talk">${esc(m.flag)}</p>
        </figcaption>
      </a>
    </figure>
  `).join("");

  DATA.modules.forEach(m => {
    const canvas = modGrid.querySelector(`canvas[data-modid="${m.id}"]`);
    if (canvas) initEmblemCanvas(canvas, m, runtime);
  });

  // Poster Wall (6 clickable SingTail telemetry pattern modules)
  const wall = document.getElementById("poster-wall");
  const CHIPS_LEFT = {
    tl: [
      { text: "MULTI-ORG TSNET · DUAL LOGIN", indent: 0 },
      { text: "网页授权登录 + Headscale 地址", indent: 1 },
      { text: "直接导入已有 ts-state 凭据目录", indent: 0 },
      { text: "各组织分目录独立运行，互不覆盖", indent: 2 }
    ],
    mr: [
      { text: "6S PUBLIC IP ROLLBACK", indent: 1 },
      { text: "每次切换出口自动实测真实公网 IP", indent: 0 },
      { text: "节点超时或断流秒级自动回退", indent: 2 }
    ],
    bl: [
      { text: "SING-BOX 1.14.3 + 7 .SRS", indent: 1 },
      { text: "国内直连含 8.128.0.0/10 修正段", indent: 0 },
      { text: "境外走加密 DNS，局域网不绕路", indent: 2 }
    ]
  };
  const CHIPS_RIGHT = {
    tl: [
      { text: "CLASH COEXISTENCE · ZERO LOOP", indent: 0 },
      { text: "默认放行 clash-verge / verge-mihomo", indent: 1 },
      { text: "支持直接点选本机运行中进程加规则", indent: 0 },
      { text: "可配置本地备用代理 127.0.0.1:7897", indent: 2 }
    ],
    mr: [
      { text: "48H SQLITE TRAFFIC LEDGER", indent: 1 },
      { text: "分钟级采样自动滚动汇总至日/月表", indent: 0 },
      { text: "按 Tailscale 组织与出口节点占比排行", indent: 2 }
    ],
    bl: [
      { text: "SANITIZED EXPORT + WIN / MAC", indent: 1 },
      { text: "导出配置强制剔除全部组织设备私钥", indent: 0 },
      { text: "Windows 安装包/便携包 · macOS DMG", indent: 2 }
    ]
  };
  const chipsHTML = data => `
    <div class="poster-chips-layer" aria-hidden="true">
      <div class="chip-group chip-group-tl">${data.tl.map(x => `<span class="poster-chip-line t-small chip-indent-${x.indent}">${esc(x.text)}</span>`).join("")}</div>
      <div class="chip-group chip-group-mr">${data.mr.map(x => `<span class="poster-chip-line t-small chip-rindent-${x.indent}">${esc(x.text)}</span>`).join("")}</div>
      <div class="chip-group chip-group-bl">${data.bl.map(x => `<span class="poster-chip-line t-small chip-indent-${x.indent}">${esc(x.text)}</span>`).join("")}</div>
    </div>
  `;

  wall.innerHTML = `
    <div class="poster-stack poster-stack-left" id="poster-left">
      <button type="button" class="poster-mod" id="mod-m1" aria-label="切换工程纹样配色 1"></button>
      <button type="button" class="poster-mod" id="mod-m3" aria-label="切换工程纹样配色 2"></button>
      <button type="button" class="poster-mod" id="mod-m4" aria-label="切换工程纹样配色 3"></button>
      ${chipsHTML(CHIPS_LEFT)}
    </div>
    <div class="poster-stack poster-stack-right" id="poster-right">
      <button type="button" class="poster-mod" id="mod-m6" aria-label="切换工程纹样配色 4"></button>
      <button type="button" class="poster-mod" id="mod-m2" aria-label="切换工程纹样配色 5"></button>
      <button type="button" class="poster-mod" id="mod-m5" aria-label="切换工程纹样配色 6"></button>
      ${chipsHTML(CHIPS_RIGHT)}
    </div>
  `;

  const PALETTES = [
    ["#161618", "#CEFF00"],
    ["#121214", "#38BDF8"],
    ["#1D1D20", "#4ADE80"],
    ["#161618", "#F5A623"],
    ["#CEFF00", "#111113"],
    ["#1D1D20", "#ECECEE"]
  ];
  const MODULES = [
    { id: "mod-m1", cols: 20, rows: 22, paletteIdx: 0, pat: PATTERNS.nodeMatrix, solidMask: (x, y) => (y < 5 ? 0 : -1) },
    { id: "mod-m2", cols: 20, rows: 22, paletteIdx: 1, pat: PATTERNS.tunnelHop, solidMask: (x, y) => (x >= 14 ? 0 : -1) },
    { id: "mod-m3", cols: 20, rows: 18, paletteIdx: 2, pat: PATTERNS.telemetryWave, solidMask: (x, y) => (y >= 13 ? 0 : -1) },
    { id: "mod-m4", cols: 20, rows: 18, paletteIdx: 3, pat: PATTERNS.crosshairGrid, solidMask: (x, y) => (y >= 14 ? 0 : -1) },
    { id: "mod-m5", cols: 20, rows: 18, paletteIdx: 4, pat: PATTERNS.nodeMatrix, solidMask: (x, y) => (x < 4 ? 0 : -1) },
    { id: "mod-m6", cols: 20, rows: 18, paletteIdx: 5, pat: PATTERNS.telemetryWave, solidMask: (x, y) => (y < 4 ? 0 : -1) }
  ];

  MODULES.forEach(mod => {
    const btn = document.getElementById(mod.id);
    if (!btn) return;
    let palIdx = mod.paletteIdx, phase = 0, timer = 0;
    const draw = () => {
      const [colA, colB] = PALETTES[palIdx];
      let rects = `<rect x="0" y="0" width="${mod.cols * 4}" height="${mod.rows * 4}" fill="${colA}"/>`;
      for (let y = 0; y < mod.rows; y++) {
        for (let x = 0; x < mod.cols; x++) {
          const mask = mod.solidMask(x, y);
          const tone = mask !== -1 ? mask : mod.pat(x, y, phase);
          if (tone === 1) rects += `<rect x="${x * 4}" y="${y * 4}" width="4" height="4" fill="${colB}"/>`;
        }
      }
      btn.innerHTML = `<svg viewBox="0 0 ${mod.cols * 4} ${mod.rows * 4}" shape-rendering="crispEdges" aria-hidden="true">${rects}</svg>`;
    };
    draw();
    btn.addEventListener("click", () => {
      palIdx = (palIdx + 1) % PALETTES.length;
      if (!runtime.state.motionOn) { phase = (phase + 1) % 8; draw(); return; }
      window.clearInterval(timer);
      let steps = 0;
      draw();
      timer = window.setInterval(() => {
        phase = (phase + 1) % 8; draw(); steps++;
        if (steps >= 4) window.clearInterval(timer);
      }, 70);
    });
    runtime.state.routeCleanup.push(() => window.clearInterval(timer));
  });

  initChassisGrid(document.getElementById("home-chassis-wrap"), runtime);
}
