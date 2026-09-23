import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import type { MediaKind } from '../lib/util'
import type { LibraryRecord } from '../types'
import { WorkModal } from './WorkModal'

export interface WorkModalItem {
  anilistId: number
  title: string
  coverImageUrl?: string | null
  genres?: string[]
  synopsis?: string
  score?: number | null
  total?: number | null
  nextAiringAt?: number | string | null
}

export interface OpenWorkModalOptions {
  item: WorkModalItem
  mediaType: MediaKind
  record: LibraryRecord | null
  onChange?: () => void
}

const WorkModalContext = createContext<((opts: OpenWorkModalOptions) => void) | null>(null)

export function WorkModalProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<{ key: number; opts: OpenWorkModalOptions } | null>(null)
  const nextKey = useRef(0)

  const open = useCallback((opts: OpenWorkModalOptions) => {
    nextKey.current += 1
    setState({ key: nextKey.current, opts })
  }, [])
  const close = useCallback(() => setState(null), [])

  useEffect(() => {
    if (!state) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [state, close])

  return (
    <WorkModalContext.Provider value={open}>
      {children}
      {state && (
        <WorkModal
          key={state.key}
          item={state.opts.item}
          mediaType={state.opts.mediaType}
          record={state.opts.record}
          onChange={state.opts.onChange}
          onClose={close}
          openRelated={open}
        />
      )}
    </WorkModalContext.Provider>
  )
}

export function useWorkModal() {
  const ctx = useContext(WorkModalContext)
  if (!ctx) throw new Error('useWorkModal must be used within WorkModalProvider')
  return ctx
}
