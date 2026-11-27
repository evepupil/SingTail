import { useEffect, useRef } from "react";
import type { TrafficReport } from "../types/contract";

interface TrafficCanvasProps {
  traffic: TrafficReport;
  theme: "dark" | "light";
}

function drawChart(
  canvas: HTMLCanvasElement,
  traffic: TrafficReport,
  theme: "dark" | "light",
): void {
  const parent = canvas.parentElement;
  if (!parent) {
    return;
  }
  const rect = parent.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) {
    return;
  }

  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.floor(rect.width * dpr);
  canvas.height = Math.floor(rect.height * dpr);

  const ctx = canvas.getContext("2d");
  if (!ctx) {
    return;
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

  const w = rect.width;
  const h = rect.height;
  ctx.clearRect(0, 0, w, h);

  const isLight = theme === "light";
  const gridColor = isLight ? "rgba(27,26,23,0.08)" : "rgba(255,255,255,0.06)";
  const labelColor = isLight ? "#625e56" : "#8a8a95";
  const downStroke = isLight ? "#2d5a00" : "#ceff00";
  const downBar = isLight ? "#4d8a00" : "#ceff00";
  const downFill = isLight ? "rgba(45,90,0,0.12)" : "rgba(206,255,0,0.14)";
  const upStroke = isLight ? "#0369a1" : "#38bdf8";

  const bottomLabelSpace = traffic.period === "live" ? 8 : 22;
  const chartH = h - bottomLabelSpace;

  ctx.strokeStyle = gridColor;
  ctx.lineWidth = 1;
  for (let i = 1; i <= 3; i += 1) {
    const y = Math.round((chartH / 4) * i) + 0.5;
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(w, y);
    ctx.stroke();
  }

  if (traffic.period === "live") {
    for (let i = 1; i < 6; i += 1) {
      const x = Math.round((w / 6) * i) + 0.5;
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, chartH);
      ctx.stroke();
    }

    const downArr = traffic.liveDownHistory;
    const upArr = traffic.liveUpHistory;
    const maxVal = Math.max(12, ...downArr, ...upArr);
    const padTop = 10;
    const usableH = chartH - padTop;

    const drawSeries = (arr: readonly number[], stroke: string, fill: string | null) => {
      if (arr.length < 2) {
        return;
      }
      const stepX = w / (arr.length - 1);
      ctx.beginPath();
      for (let idx = 0; idx < arr.length; idx += 1) {
        const v = arr[idx] ?? 0;
        const x = idx * stepX;
        const y = padTop + usableH - (v / maxVal) * usableH;
        if (idx === 0) {
          ctx.moveTo(x, y);
        } else {
          ctx.lineTo(x, y);
        }
      }
      ctx.strokeStyle = stroke;
      ctx.lineWidth = 1.7;
      ctx.stroke();

      if (fill) {
        ctx.lineTo(w, chartH);
        ctx.lineTo(0, chartH);
        ctx.closePath();
        ctx.fillStyle = fill;
        ctx.fill();
      }
    };

    drawSeries(downArr, downStroke, downFill);
    drawSeries(upArr, upStroke, null);
    return;
  }

  const labels = traffic.labels;
  const n = labels.length;
  if (n === 0) {
    return;
  }

  const totals = labels.map((_, i) => (traffic.downSeries[i] ?? 0) + (traffic.upSeries[i] ?? 0));
  const maxTotal = Math.max(1, ...totals) * 1.15;
  const padX = 18;
  const slotW = (w - padX * 2) / n;
  const barW = Math.min(28, Math.max(10, Math.floor(slotW * 0.48)));

  ctx.font = '10.5px "Geist Mono", Consolas, monospace';
  ctx.textAlign = "center";

  for (let i = 0; i < n; i += 1) {
    const downV = traffic.downSeries[i] ?? 0;
    const upV = traffic.upSeries[i] ?? 0;
    const downH = Math.max(2, (downV / maxTotal) * (chartH - 12));
    const upH = Math.max(1, (upV / maxTotal) * (chartH - 12));

    const cx = padX + slotW * i + slotW / 2;
    const x = Math.round(cx - barW / 2);
    const yDown = Math.round(chartH - downH);
    const yUp = Math.round(yDown - upH - 1);

    ctx.fillStyle = downBar;
    ctx.fillRect(x, yDown, barW, downH);

    ctx.fillStyle = upStroke;
    ctx.fillRect(x, yUp, barW, upH);

    ctx.fillStyle = labelColor;
    ctx.fillText(labels[i] ?? "", cx, h - 6);
  }
}

export function TrafficCanvas({ traffic, theme }: TrafficCanvasProps) {
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) {
      return;
    }

    const render = () => drawChart(canvas, traffic, theme);
    const raf = requestAnimationFrame(render);

    const observer = new ResizeObserver(() => {
      render();
    });
    observer.observe(wrap);

    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
    };
  }, [traffic, theme]);

  return (
    <div ref={wrapRef} className="sparkline-canvas-wrap">
      <canvas ref={canvasRef} id="sparkline-canvas" className="sparkline-canvas" />
    </div>
  );
}
