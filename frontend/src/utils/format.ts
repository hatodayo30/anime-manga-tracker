export function formatCountdown(ts: number | null): string {
  if (!ts) return '';
  const diff = ts - Date.now();
  if (diff <= 0) return '放送中';
  const hours = Math.ceil(diff / 3600000);
  if (hours < 24) return `あと${hours}時間`;
  return `あと${Math.ceil(hours / 24)}日`;
}
