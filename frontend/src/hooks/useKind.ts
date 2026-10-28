import { useCallback, useState } from 'react'
import type { MediaKind } from '../lib/util'

const KIND_STORAGE_KEY = 'kind'

function readKind(): MediaKind {
  try {
    return localStorage.getItem(KIND_STORAGE_KEY) === 'manga' ? 'manga' : 'anime'
  } catch {
    return 'anime'
  }
}

// ログイン・サインアップの直後はアニメから始める。前回の利用で漫画に切り替えたままだと、
// ログインするたびに漫画のホームが開いてしまうため。ログイン中の画面遷移では覚えた種別を使う。
export function resetKindToAnime(): void {
  try {
    localStorage.setItem(KIND_STORAGE_KEY, 'anime')
  } catch {
    // 保存できなければ readKind が既定のアニメを返すので無視する
  }
}

// アニメ/漫画の表示切り替え。localStorageで全画面に共有する。
export function useKind(): [MediaKind, (kind: MediaKind) => void] {
  const [kind, setKindState] = useState<MediaKind>(readKind)

  const setKind = useCallback((next: MediaKind) => {
    setKindState(next)
    try {
      localStorage.setItem(KIND_STORAGE_KEY, next)
    } catch {
      // ignore
    }
  }, [])

  return [kind, setKind]
}
