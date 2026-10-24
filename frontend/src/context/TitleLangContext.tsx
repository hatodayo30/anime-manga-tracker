import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'

// 作品名の表示言語（BACKLOG 6）。'ja' はサーバーが選んだ title（韓国・中国作品は別名の日本語版や英語）、
// 'en' は titleEn をそのまま出す。どちらを出すかの判断はサーバーに置き、ここは切り替えるだけ。
export type TitleLang = 'ja' | 'en'

const TITLE_LANG_STORAGE_KEY = 'titleLang'

function readTitleLang(): TitleLang {
  try {
    return localStorage.getItem(TITLE_LANG_STORAGE_KEY) === 'en' ? 'en' : 'ja'
  } catch {
    return 'ja'
  }
}

interface TitledItem {
  title: string
  titleEn?: string | null
}

interface TitleLangContextValue {
  lang: TitleLang
  setLang: (lang: TitleLang) => void
  // titleEn が無いもの（ライブラリの記録など、保存済みの日本語名しか持たないもの）は title に戻す
  displayTitle: (item: TitledItem) => string
}

const TitleLangContext = createContext<TitleLangContextValue | null>(null)

// useKind と違い Context にするのは、サイドバーの切り替えを開いている画面とモーダルへ同時に効かせるため。
// useKind は画面ごとに state を持つので、別の画面へ移るまで反映されない。
export function TitleLangProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<TitleLang>(readTitleLang)

  const setLang = useCallback((next: TitleLang) => {
    setLangState(next)
    try {
      localStorage.setItem(TITLE_LANG_STORAGE_KEY, next)
    } catch {
      // プライベートブラウズなどで保存できなくても、このタブの中では切り替わる
    }
  }, [])

  const displayTitle = useCallback(
    (item: TitledItem) => (lang === 'en' && item.titleEn ? item.titleEn : item.title),
    [lang],
  )

  return <TitleLangContext.Provider value={{ lang, setLang, displayTitle }}>{children}</TitleLangContext.Provider>
}

export function useTitleLang() {
  const ctx = useContext(TitleLangContext)
  if (!ctx) throw new Error('useTitleLang must be used within TitleLangProvider')
  return ctx
}
