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
