export interface SparklineGeometry {
  linePath: string;
  areaPath: string;
  lastX: number;
  lastY: number;
  hasData: boolean;
}

export function buildSparklineGeometry(
  rawValues: (number | null | undefined)[],
  minFixed?: number,
  maxFixed?: number,
  width = 120,
  height = 36
): SparklineGeometry {
  const valid = rawValues.filter((v): v is number => typeof v === 'number' && Number.isFinite(v));
  if (valid.length === 0) {
    return { linePath: '', areaPath: '', lastX: 0, lastY: height - 2, hasData: false };
  }

  let minV = minFixed ?? Math.min(...valid);
  let maxV = maxFixed ?? Math.max(...valid);
  if (maxV - minV < 1) {
    const pad = maxV === 0 ? 10 : Math.max(2, maxV * 0.15);
    minV = Math.max(0, minV - pad);
    maxV = maxV + pad;
  }

  const padY = 4;
  const usableH = Math.max(6, height - padY * 2);
  const points: { x: number; y: number }[] = [];

  const totalSlots = rawValues.length;
  for (let i = 0; i < totalSlots; i++) {
    const val = rawValues[i];
    if (typeof val !== 'number' || !Number.isFinite(val)) continue;
    const x = totalSlots <= 1 ? width : (i / (totalSlots - 1)) * width;
    const norm = Math.max(0, Math.min(1, (val - minV) / (maxV - minV)));
    const y = padY + (1 - norm) * usableH;
    points.push({ x: Number(x.toFixed(1)), y: Number(y.toFixed(1)) });
  }

  if (points.length === 0) {
    return { linePath: '', areaPath: '', lastX: 0, lastY: height - 2, hasData: false };
  }

  if (points.length === 1) {
    const p = points[0];
    const linePath = `M 0 ${p.y} L ${width} ${p.y}`;
    const areaPath = `M 0 ${p.y} L ${width} ${p.y} L ${width} ${height} L 0 ${height} Z`;
    return { linePath, areaPath, lastX: width, lastY: p.y, hasData: true };
  }

  const lineCmds = points.map((p, idx) => `${idx === 0 ? 'M' : 'L'} ${p.x} ${p.y}`);
  const first = points[0];
  const last = points[points.length - 1];
  const linePath = lineCmds.join(' ');
  const areaPath = `${linePath} L ${last.x} ${height} L ${first.x} ${height} Z`;

  return {
    linePath,
    areaPath,
    lastX: last.x,
    lastY: last.y,
    hasData: true,
  };
}

export function formatMB(mb: number): string {
  if (!Number.isFinite(mb) || mb <= 0) return '0 MB';
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${Math.round(mb)} MB`;
}
