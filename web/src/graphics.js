import { DATA } from "./data.js";

export const TRACK_COLORS = [
  DATA.tokens.lime,
  DATA.tokens.cyan,
  DATA.tokens.green,
  DATA.tokens.amber
];

export const TRAIL_COLORS = [
  DATA.tokens.lime,
  DATA.tokens.cyan,
  DATA.tokens.green,
  DATA.tokens.amber,
  DATA.tokens.paper
];

export const STAMP_PALETTE = [
  { id: "0", name: "Charcoal", hex: DATA.tokens.charcoal },
  { id: "1", name: "Paper", hex: DATA.tokens.paper },
  { id: "2", name: "Acid Lime", hex: DATA.tokens.lime },
  { id: "3", name: "Telemetry Cyan", hex: DATA.tokens.cyan },
  { id: "4", name: "Direct Green", hex: DATA.tokens.green },
  { id: "5", name: "DERP Amber", hex: DATA.tokens.amber },
  { id: "6", name: "Reject Red", hex: DATA.tokens.red },
  { id: "7", name: "Rail Dark", hex: DATA.tokens.rail },
  { id: "8", name: "Sand", hex: DATA.tokens.sand }
];

export function esc(str) {
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export function hashStr(s) {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export function makeRng(seed) {
  let a = seed >>> 0;
  return function () {
    a |= 0;
    a = (a + 0x6D2B79F5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function cubicBezier(x1, y1, x2, y2) {
  return function (t) {
    if (t <= 0) return 0;
    if (t >= 1) return 1;
    let lo = 0, hi = 1, u = t;
    for (let i = 0; i < 12; i++) {
      u = (lo + hi) * 0.5;
      const mu = 1 - u;
      const x = 3 * mu * mu * u * x1 + 3 * mu * u * u * x2 + u * u * u;
      if (x < t) lo = u;
      else hi = u;
    }
    const mu = 1 - u;
    return 3 * mu * mu * u * y1 + 3 * mu * u * u * y2 + u * u * u;
  };
}

export const easeMorph = cubicBezier(0.7, 0, 0.3, 1);
export const easeAssemble = cubicBezier(0.2, 0, 0, 1);

export function layoutGlyphLines(lines, gapRows = 2, useBold = false) {
  const set = useBold ? DATA.glyphsBold : DATA.glyphs;
  let maxCols = 0;
  const cells = [];
  let rowOffset = 0;
  for (let l = 0; l < lines.length; l++) {
    const str = lines[l];
    let colOffset = 0;
    for (let i = 0; i < str.length; i++) {
      const ch = str[i];
      const g = set[ch] || set[" "];
      const w = g[0].length;
      for (let r = 0; r < 9; r++) {
        for (let c = 0; c < w; c++) {
          if (g[r][c] === "#") {
            cells.push({ x: colOffset + c, y: rowOffset + r });
          }
        }
      }
      colOffset += w + (i < str.length - 1 ? 1 : 0);
    }
    if (colOffset > maxCols) maxCols = colOffset;
    rowOffset += 9 + (l < lines.length - 1 ? gapRows : 0);
  }
  return { cols: maxCols, rows: rowOffset, cells };
}

export function svgBitmapText(str, cellH, fill, opts = {}) {
  const layout = layoutGlyphLines(Array.isArray(str) ? str : [str], 2, Boolean(opts.bold));
  const vbW = layout.cols * 5;
  const vbH = layout.rows * 4;
  const color = fill || "currentColor";
  let rects = "";
  if (opts.perCell) {
    for (let i = 0; i < layout.cells.length; i++) {
      const cell = layout.cells[i];
      rects += `<rect x="${cell.x * 5}" y="${cell.y * 4}" width="5" height="4" data-gx="${cell.x}" data-gy="${cell.y}"/>`;
    }
  } else {
    const rowMap = new Map();
    for (let i = 0; i < layout.cells.length; i++) {
      const cell = layout.cells[i];
      if (!rowMap.has(cell.y)) rowMap.set(cell.y, []);
      rowMap.get(cell.y).push(cell.x);
    }
    rowMap.forEach((xs, y) => {
      xs.sort((a, b) => a - b);
      let start = xs[0], prev = xs[0];
      for (let k = 1; k <= xs.length; k++) {
        if (k < xs.length && xs[k] === prev + 1) {
          prev = xs[k];
        } else {
          const span = prev - start + 1;
          rects += `<rect x="${start * 5}" y="${y * 4}" width="${span * 5}" height="4.04"/>`;
          if (k < xs.length) {
            start = xs[k];
            prev = xs[k];
          }
        }
      }
    });
  }
  const wAttr = opts.fluid ? `width="100%"` : `width="${layout.cols * cellH * 1.25}" height="${layout.rows * cellH}"`;
  const cls = opts.className ? ` class="${esc(opts.className)}"` : "";
  const aria = opts.ariaHidden ? ` aria-hidden="true"` : "";
  return `<svg${cls} viewBox="0 0 ${vbW} ${vbH}" ${wAttr} shape-rendering="crispEdges" fill="${color}"${aria}>${rects}</svg>`;
}

export function svgPixelIcon(name, size = 14) {
  const grids = {
    plus: ["..#..", "..#..", "#####", "..#..", "..#.."],
    minus: [".....", ".....", "#####", ".....", "....."],
    check: ["....#", "...#.", "#.#..", ".#...", "....."]
  };
  const g = grids[name] || grids.plus;
  let rects = "";
  for (let y = 0; y < 5; y++) {
    for (let x = 0; x < 5; x++) {
      if (g[y][x] === "#") rects += `<rect x="${x}" y="${y}" width="1" height="1"/>`;
    }
  }
  return `<svg width="${size}" height="${size}" viewBox="0 0 5 5" shape-rendering="crispEdges" fill="currentColor" aria-hidden="true">${rects}</svg>`;
}

/** SingTail-exclusive architectural & network telemetry pixel patterns (replacing textile weaves). */
export const PATTERNS = {
  nodeMatrix: (x, y, phase) => {
    const px = (x + phase) % 8;
    const py = (y + phase) % 8;
    const isNode = (px === 1 || px === 4) && (py === 1 || py === 4);
    const isDiagLink = (px + py === 7) && px >= 2 && px <= 5;
    return (isNode || isDiagLink) ? 1 : 0;
  },
  telemetryWave: (x, y, phase) => {
    const waveA = Math.round(4 + Math.sin((x + phase) * 0.55) * 2.5);
    const waveB = Math.round(11 + Math.cos((x + phase) * 0.45) * 2.5);
    return (Math.abs((y % 16) - waveA) <= 1 || Math.abs((y % 16) - waveB) <= 0.6) ? 1 : 0;
  },
  crosshairGrid: (x, y, phase) => {
    const px = (x + phase) % 7;
    const py = (y + phase) % 7;
    const cross = (px === 3 && py >= 1 && py <= 5) || (py === 3 && px >= 1 && px <= 5);
    const corner = px === 0 && py === 0;
    return (cross || corner) ? 1 : 0;
  },
  tunnelHop: (x, y, phase) => {
    const step = (x + phase) % 8;
    const row = y % 6;
    const lane = (row === 1 && step < 5) || (row === 4 && step >= 3);
    const hop = step === 4 && row >= 1 && row <= 4;
    return (lane || hop) ? 1 : 0;
  }
};

/** Deterministic 32x32 pixel hardware & network emblems for SingTail's 8 core capabilities. */
export function buildModuleEmblemRows(modId) {
  const rows = [];
  for (let y = 0; y < 32; y++) {
    let row = "";
    for (let x = 0; x < 32; x++) {
      const dx = x - 15.5;
      const dy = y - 15.5;
      const ax = Math.abs(dx);
      const ay = Math.abs(dy);
      let c = "0";
      if (modId === "tailnet") {
        // 3x3 Exit-Node Matrix motif matching SingTail Logo
        const gx = Math.floor((x - 5) / 8);
        const gy = Math.floor((y - 5) / 8);
        const lx = (x - 5) % 8;
        const ly = (y - 5) % 8;
        if (x >= 5 && x < 27 && y >= 5 && y < 27 && lx < 6 && ly < 6) {
          if (gx === 2 && gy === 0) c = "3";
          else if ((gx === 0 && gy === 2) || (gx === 1 && gy === 2) || (gx === 1 && gy === 1)) c = "1";
          else if (lx === 0 || lx === 5 || ly === 0 || ly === 5) c = "2";
        }
        if ((x >= 11 && x <= 12 && y >= 23 && y <= 24) || (x >= 15 && x <= 16 && y >= 19 && y <= 20)) c = "1";
        if (x >= 17 && x <= 22 && Math.abs((x - 17) - (14 - y)) <= 1) c = "3";
      } else if (modId === "rollback") {
        // Dual probe arrow + safe rollback loop
        if (ax <= 11 && ay <= 11 && (ax >= 9 || ay >= 9) && !(dx > 4 && dy < -4)) c = "2";
        if (dx >= 4 && dx <= 11 && dy <= -4 && dy >= -11) c = "3";
        if (Math.abs(dx + dy) <= 1.8 && ax <= 8) c = "1";
        if (ax <= 2.5 && ay <= 2.5) c = "3";
      } else if (modId === "singbox") {
        // Nested TUN packet prism box
        if (ax <= 12 && ay <= 12 && (ax >= 10 || ay >= 10)) c = "1";
        if (ax <= 7 && ay <= 7 && (ax >= 5 || ay >= 5)) c = "2";
        if (ax <= 2.5 && ay <= 2.5) c = "3";
      } else if (modId === "coexist") {
        // Twin non-colliding parallel bypass lanes
        if (y >= 7 && y <= 11 && x >= 4 && x <= 27) c = "1";
        if (y >= 20 && y <= 24 && x >= 4 && x <= 27) c = "2";
        if (y >= 14 && y <= 17 && ((x >= 4 && x <= 12) || (x >= 19 && x <= 27))) c = "3";
      } else if (modId === "process") {
        // Desktop app window + process hook crosshair
        if (x >= 5 && x <= 26 && y >= 6 && y <= 25 && (x <= 6 || x >= 25 || y <= 9 || y >= 24)) c = "1";
        if (y === 7 && (x === 8 || x === 11 || x === 14)) c = "3";
        if (x >= 9 && x <= 22 && (y === 14 || y === 18)) c = "2";
        if (x >= 19 && x <= 22 && y === 14) c = "3";
      } else if (modId === "srs") {
        // Layered rule-set stack slabs
        if (ax <= 11 && ((y >= 6 && y <= 9) || (y >= 14 && y <= 17) || (y >= 22 && y <= 25))) c = "1";
        if (x >= 7 && x <= 10 && (y === 7 || y === 8 || y === 15 || y === 16 || y === 23 || y === 24)) c = "3";
        if (x >= 14 && x <= 23 && (y === 15 || y === 16)) c = "2";
      } else if (modId === "stats") {
        // Stepped SQLite telemetry waveform bars
        const bars = [6, 10, 14, 18, 13, 21, 16, 11];
        const bi = Math.floor((x - 4) / 3);
        if (bi >= 0 && bi < bars.length && (x - 4) % 3 < 2 && y <= 26 && y >= 26 - bars[bi]) {
          c = bars[bi] >= 18 ? "3" : (bi % 2 === 0 ? "1" : "2");
        }
        if (y === 27 && x >= 4 && x <= 27) c = "2";
      } else {
        // Tray & Service daemon shield pulse
        if (ax <= 10 && y >= 5 && y <= 20 && (ax >= 8 || y <= 7)) c = "1";
        if (y > 20 && y <= 26 && ax <= 10 - (y - 20) * 1.4 && ax >= 8 - (y - 20) * 1.4) c = "1";
        if (ax <= 3 && y >= 11 && y <= 17) c = "3";
        if (y >= 13 && y <= 15 && ax <= 5) c = "2";
      }
      row += c;
    }
    rows.push(row);
  }
  return rows;
}

/** Live rule text validator matching SingTail's internal/rules parser format. */
export function validateRuleLines(text) {
  const lines = String(text || "").split(/\r?\n/);
  const parsed = [];
  const errors = [];
  const validTypes = new Set([
    "DOMAIN",
    "DOMAIN-SUFFIX",
    "DOMAIN-KEYWORD",
    "IP-CIDR",
    "IP-CIDR6",
    "PROCESS-NAME",
    "RULE-SET"
  ]);
  const validActions = new Set(["DIRECT", "EXIT", "PROXY", "REJECT", "BACKUP"]);

  lines.forEach((raw, idx) => {
    const trimmed = raw.trim();
    if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("//") || trimmed.startsWith(";")) return;
    const parts = trimmed.split(",").map(s => s.trim());
    if (parts.length < 3) {
      errors.push(`第 ${idx + 1} 行格式不足 3 段：${trimmed}`);
      return;
    }
    const kind = parts[0].toUpperCase();
    const val = parts[1];
    const action = parts[2].toUpperCase();
    if (!validTypes.has(kind)) {
      errors.push(`第 ${idx + 1} 行不支持的匹配类型：${parts[0]}`);
      return;
    }
    if (!val) {
      errors.push(`第 ${idx + 1} 行匹配值为空`);
      return;
    }
    if (!validActions.has(action)) {
      errors.push(`第 ${idx + 1} 行未知动作：${parts[2]}（可选 DIRECT / EXIT / BACKUP / REJECT）`);
      return;
    }
    parsed.push({ line: idx + 1, kind, value: val, action });
  });

  return { parsed, errors };
}

export async function copyText(text, btn) {
  const prev = btn ? btn.textContent : "";
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    ta.remove();
  }
  if (btn) {
    btn.textContent = "已复制";
    window.setTimeout(() => {
      btn.textContent = prev;
    }, 1500);
  }
}
