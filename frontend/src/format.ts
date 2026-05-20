export const num = (value: unknown): number => {
  const n = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(n) ? n : 0;
};

export const text = (value: unknown, fallback = ''): string => {
  return typeof value === 'string' && value.trim() !== '' ? value : fallback;
};

export const fmt = (value: unknown) => {
  const n = num(value);
  if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
  return n.toString();
};

export const fmtCost = (value: unknown) => {
  const n = num(value);
  if (n !== 0 && Math.abs(n) < 0.01) return '$' + n.toFixed(4);
  return '$' + n.toFixed(2);
};
export const fmtPercent = (value: unknown) => num(value).toFixed(1) + '%';
