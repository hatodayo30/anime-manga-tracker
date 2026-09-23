// "/" 始まりかつ "//" では始まらないパスのみ許可（外部サイトへのオープンリダイレクト対策）
export function safeNextPath(search: string): string | null {
  const next = new URLSearchParams(search).get('next')
  if (next && next.startsWith('/') && !next.startsWith('//')) return next
  return null
}
