// 「最近の検索」（作品名）を端末に残す。サーバーの変更は要らない。
// localStorage は使えない環境（プライベートブラウズなど）で例外を投げることがあるので、必ず try/catch で包む。

const STORAGE_KEY = 'miroku.recentTitleSearches'
export const RECENT_MAX = 5

export function loadRecent(): string[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    const parsed: unknown = raw ? JSON.parse(raw) : []
    if (!Array.isArray(parsed)) return []
    return parsed.filter((v): v is string => typeof v === 'string').slice(0, RECENT_MAX)
  } catch {
    return []
  }
}

export function saveRecent(list: string[]): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(list.slice(0, RECENT_MAX)))
  } catch {
    // 保存できなくても画面は動くので無視する
  }
}

/** 先頭に足す（同じ言葉は 1 つにまとめる） */
export function pushRecent(list: string[], query: string): string[] {
  const q = query.trim()
  if (!q) return list
  return [q, ...list.filter((v) => v !== q)].slice(0, RECENT_MAX)
}

export function removeRecent(list: string[], query: string): string[] {
  return list.filter((v) => v !== query)
}
