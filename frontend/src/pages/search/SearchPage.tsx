// 検索・追加画面（案8）
// ・「作品名で探す」と「内容で探す」を、枠の分かれた 2 つのボタンで切り替える。欄そのものが入れ替わる
// ・作品名: 打つと 500ms 待って自動で検索。Enter と「検索」ボタンで即時。上限 100 文字
// ・内容:   Enter か「探す」ボタンで確定したときだけ AI を呼ぶ（費用のため）。IME の確定の Enter では呼ばない。上限 200 文字
// ・2 つの欄は文字と結果を別々に持つ。切り替えても混ざらない
// ・結果のポスターは 1 枚ずつ少し下からフェードで入る
// ・積み作品などの棚は出さない（入力欄と 2 つの探し方だけ）
// API の呼び出しは props で受け取る。つなぎ込みは pages/SearchPage.tsx。

import { useCallback, useEffect, useRef, useState, type CSSProperties, type FormEvent, type KeyboardEvent } from 'react'
import { PosterCard } from '../../components/PosterCard'
import type { AiConditions, ContentSearchResult, FallbackReason, MediaKind, WorkSummary } from './searchTypes'
import { loadRecent, pushRecent, removeRecent, saveRecent } from './recentSearches'
import './search-redesign.css'

const TITLE_MAX = 100
const CONTENT_MAX = 200
const TITLE_DEBOUNCE_MS = 500
const STAGGER_CAP = 10 // 11 枚目以降は同じ遅れで出す（待たせすぎない）
const KIND_LABEL: Record<MediaKind, string> = { anime: 'アニメ', manga: '漫画' }
const CONTENT_EXAMPLES = ['異世界で料理する漫画', '泣ける部活もののアニメ', '大人が主人公の静かな日常もの']

type Mode = 'title' | 'content'

export interface SearchPageProps {
  /** 画面右上の種別（アニメ / 漫画） */
  kind: MediaKind
  onKindChange: (kind: MediaKind) => void
  /** 内容で探すはログインが必要 */
  isLoggedIn: boolean
  /** ログインの案内のボタンを押したとき */
  onLoginClick: () => void
  /** 作品名で検索する（今の API）。signal で中断できるようにする */
  searchByTitle: (query: string, kind: MediaKind, signal: AbortSignal) => Promise<WorkSummary[]>
  /** 内容で探す（今の API）。従来検索に落ちたときは fallback に理由を入れて返す */
  searchByContent: (text: string, kind: MediaKind, signal: AbortSignal) => Promise<ContentSearchResult>
  /** 最初に入れておく作品名（「新着」の新刊から「作品を探して記録する」で来たとき）。入っていればすぐ検索する */
  initialTitleQuery?: string
  /** ポスターを押したとき（作品モーダルを開く） */
  onOpenWork: (work: WorkSummary) => void
  /**
   * ライブラリに入っているときのステータス表示（例: 「見た」「読みたい」）。入っていなければ null。
   * 結果に持たせず描画のたびに引くのは、作品モーダルで追加・変更したあと検索し直さずにバッジを変えるため。
   */
  statusLabelFor: (work: WorkSummary) => string | null
}

/** IME の変換を確定する Enter かどうか */
function isImeEnter(e: KeyboardEvent): boolean {
  return e.nativeEvent.isComposing || e.keyCode === 229
}

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
}

/** アニメーションの遅れを CSS 変数で渡す */
function staggerStyle(index: number): CSSProperties {
  return { ['--srch-i' as string]: Math.min(index, STAGGER_CAP) } as CSSProperties
}

export function SearchPage({ kind, onKindChange, isLoggedIn, onLoginClick, searchByTitle, searchByContent, initialTitleQuery, onOpenWork, statusLabelFor }: SearchPageProps) {
  const [mode, setMode] = useState<Mode>('title')
  const [showLogin, setShowLogin] = useState(false)

  // ----- 作品名 -----
  const [titleQuery, setTitleQuery] = useState(initialTitleQuery ?? '')
  // query は検索した言葉。見出しに打っている途中の文字ではなく、結果の元になった言葉を出すため
  const [titleResult, setTitleResult] = useState<{ query: string; works: WorkSummary[] } | null>(null)
  const [titleError, setTitleError] = useState(false)
  // 作品名の検索の応答待ち。普段は 1 秒もかからないが、AniList のレート制限や Jikan での補完で数秒かかることがあり、
  // その間に何も出ないと「見つからなかった」と取り違える（2026-10-25 に実際に起きた）
  const [titleLoading, setTitleLoading] = useState(false)
  const [titleRun, setTitleRun] = useState(0) // 結果が変わるたびに増やし、フェードを最初から流す
  const [recent, setRecent] = useState<string[]>(() => loadRecent())
  // IME で変換している間は自動検索しない（変換前のかなで AniList を呼ばない。レートはプロセス全体で 24 req/分）。
  // 確定したら composeEnd を増やして自動検索をかけ直す
  const composingRef = useRef(false)
  const [composeEnd, setComposeEnd] = useState(0)
  const titleAbort = useRef<AbortController | null>(null)
  const lastTitleKey = useRef('')

  // ----- 内容 -----
  const [contentText, setContentText] = useState('')
  const [contentResult, setContentResult] = useState<ContentSearchResult | null>(null)
  const [contentLoading, setContentLoading] = useState(false)
  const [contentError, setContentError] = useState(false)
  const [elapsed, setElapsed] = useState(0)
  const [contentRun, setContentRun] = useState(0)
  /** 検索中だけ入る。連打の判定にも使う（state だと同じ描画の間に 2 回押されたときに間に合わない） */
  const contentAbort = useRef<AbortController | null>(null)
  const elapsedTimer = useRef<number | null>(null)
  /** 同じ文・同じ種別でもう一度押されたら、AI を呼ばずに前の結果を使う */
  const contentCache = useRef<{ key: string; result: ContentSearchResult } | null>(null)

  const stopElapsed = () => {
    if (elapsedTimer.current !== null) {
      window.clearInterval(elapsedTimer.current)
      elapsedTimer.current = null
    }
  }

  useEffect(
    () => () => {
      titleAbort.current?.abort()
      contentAbort.current?.abort()
      stopElapsed()
    },
    [],
  )

  // ===== 作品名の検索 =====
  const runTitle = useCallback(
    async (raw: string, force = false) => {
      const q = raw.trim()
      const key = `${kind}:${q}`
      if (!q) {
        titleAbort.current?.abort()
        lastTitleKey.current = ''
        setTitleResult(null)
        setTitleError(false)
        setTitleLoading(false)
        return
      }
      if (!force && key === lastTitleKey.current) return // 自動検索と Enter が重なったときの二重呼び出しを防ぐ
      lastTitleKey.current = key
      titleAbort.current?.abort()
      const controller = new AbortController()
      titleAbort.current = controller
      setTitleLoading(true)
      try {
        const works = await searchByTitle(q, kind, controller.signal)
        if (controller.signal.aborted) return
        setTitleResult({ query: q, works })
        setTitleError(false)
        setTitleRun((n) => n + 1)
      } catch (err) {
        if (isAbortError(err) || controller.signal.aborted) return
        lastTitleKey.current = ''
        setTitleError(true)
      } finally {
        // 新しい検索に中断されたときは、その検索がまだ応答待ちなので消さない
        if (!controller.signal.aborted) setTitleLoading(false)
      }
    },
    [kind, searchByTitle],
  )

  // 打つたびに 500ms 待って自動で検索（種別を変えたときも同じ）
  useEffect(() => {
    const id = window.setTimeout(() => {
      if (!composingRef.current) void runTitle(titleQuery)
    }, TITLE_DEBOUNCE_MS)
    return () => window.clearTimeout(id)
  }, [titleQuery, runTitle, composeEnd])

  /** Enter・検索ボタン・最近の検索から確定したときだけ「最近の検索」に残す（打っている途中の文字は残さない） */
  const commitTitle = (raw: string) => {
    void runTitle(raw)
    const q = raw.trim()
    if (!q) return
    setRecent((list) => {
      const next = pushRecent(list, q)
      saveRecent(next)
      return next
    })
  }

  const onTitleSubmit = (e: FormEvent) => {
    e.preventDefault()
    commitTitle(titleQuery)
  }

  const onTitleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') return
    if (isImeEnter(e)) return
    e.preventDefault()
    commitTitle(titleQuery)
  }

  const pickRecent = (q: string) => {
    setTitleQuery(q)
    commitTitle(q)
  }

  const dropRecent = (q: string) => {
    setRecent((list) => {
      const next = removeRecent(list, q)
      saveRecent(next)
      return next
    })
  }

  // ===== 内容で探す =====
  const runContent = async (force = false) => {
    const text = contentText.trim()
    if (!isLoggedIn) {
      setShowLogin(true)
      return
    }
    if (!text || contentAbort.current) return // 検索中は二重に AI を呼ばない
    const key = `${kind}:${text}`
    if (!force && contentCache.current?.key === key) {
      setContentResult(contentCache.current.result)
      setContentError(false)
      setContentRun((n) => n + 1)
      return
    }

    const controller = new AbortController()
    contentAbort.current = controller
    setContentLoading(true)
    setContentError(false)
    setContentResult(null)
    setElapsed(0)
    stopElapsed()
    elapsedTimer.current = window.setInterval(() => setElapsed((s) => s + 1), 1000)

    try {
      const result = await searchByContent(text, kind, controller.signal)
      if (controller.signal.aborted) return
      // AI が失敗した・使えない結果は覚えない（もう一度押したら AI を呼び直す）
      contentCache.current = result.fallback === 'ai_failed' || result.fallback === 'ai_unavailable' ? null : { key, result }
      setContentResult(result)
      setContentRun((n) => n + 1)
    } catch (err) {
      if (isAbortError(err) || controller.signal.aborted) return
      setContentError(true)
    } finally {
      if (contentAbort.current === controller) {
        contentAbort.current = null
        stopElapsed()
        setContentLoading(false)
      }
    }
  }

  /** 画面側の中断。サーバーで AI がもう動いていれば費用は掛かる */
  const cancelContent = () => {
    contentAbort.current?.abort()
    contentAbort.current = null
    stopElapsed()
    setContentLoading(false)
  }

  const onContentSubmit = (e: FormEvent) => {
    e.preventDefault()
    void runContent()
  }

  const onContentKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key !== 'Enter' || e.shiftKey) return
    if (isImeEnter(e)) return // IME の確定では検索しない
    e.preventDefault() // 改行させない
    void runContent()
  }

  // ===== 切り替え =====
  const pickMode = (next: Mode) => {
    if (next === 'content' && !isLoggedIn) {
      setShowLogin(true) // 切り替えずに案内だけ出す
      return
    }
    setShowLogin(false)
    setMode(next)
  }

  const isTitle = mode === 'title'

  return (
    <div className="srch-screen">
      <div className="srch-head">
        <h2 style={{ margin: 0 }}>検索・追加</h2>
        <div className="seg" style={{ flex: 'none', marginBottom: 0 }} role="group" aria-label="種別">
          {(['anime', 'manga'] as const).map((k) => (
            <button key={k} type="button" className={`seg-opt${kind === k ? ' checked' : ''}`} aria-pressed={kind === k} onClick={() => onKindChange(k)}>
              {KIND_LABEL[k]}
            </button>
          ))}
        </div>
      </div>

      <div className="srch-modes" role="group" aria-label="探し方">
        <button type="button" className={`srch-mode${isTitle ? ' is-on' : ''}`} aria-pressed={isTitle} onClick={() => pickMode('title')}>
          <span className="srch-mode-title">
            <SearchIcon size={15} />
            作品名で探す
          </span>
          <span className="srch-mode-sub">タイトルがわかるとき</span>
        </button>
        <button type="button" className={`srch-mode${!isTitle ? ' is-on' : ''}`} aria-pressed={!isTitle} onClick={() => pickMode('content')}>
          <span className="srch-mode-title">内容で探す</span>
          <span className="srch-mode-sub">どんな作品かを文章で</span>
        </button>
      </div>

      {showLogin && (
        <div className="srch-notice srch-notice-info srch-notice-top srch-fade" role="status">
          <LockIcon />
          <div>
            <div className="srch-notice-title">内容で探すには、ログインが必要です</div>
            <div className="srch-notice-body">「異世界で料理する漫画」のように、内容を文章で書いて探せます。作品名での検索はこのまま使えます。</div>
            <div className="srch-notice-actions">
              <button type="button" className="btn btn-primary" onClick={onLoginClick}>
                ログイン
              </button>
              <button type="button" className="btn btn-secondary" onClick={() => setShowLogin(false)}>
                閉じる
              </button>
            </div>
          </div>
        </div>
      )}

      {isTitle ? (
        <section className="srch-swap-in" key="title" aria-label="作品名で探す">
          <form className="srch-title-row" role="search" onSubmit={onTitleSubmit}>
            <div className="srch-title-field">
              <SearchIcon size={18} />
              <label className="srch-sr-only" htmlFor="srch-title-input">
                作品名
              </label>
              <input
                id="srch-title-input"
                className="input"
                type="search"
                enterKeyHint="search"
                autoComplete="off"
                maxLength={TITLE_MAX}
                placeholder={`${KIND_LABEL[kind]}のタイトル`}
                value={titleQuery}
                onChange={(e) => setTitleQuery(e.target.value)}
                onCompositionStart={() => {
                  composingRef.current = true
                }}
                onCompositionEnd={() => {
                  composingRef.current = false
                  setComposeEnd((n) => n + 1)
                }}
                onKeyDown={onTitleKeyDown}
              />
            </div>
            <button type="submit" className="btn srch-btn-accent srch-title-go">
              検索
            </button>
          </form>

          {!titleQuery.trim() && recent.length > 0 && (
            <div className="srch-chips srch-fade">
              <span className="text-muted srch-chips-label">最近の検索</span>
              {recent.map((q) => (
                <span className="srch-recent" key={q}>
                  <button type="button" className="srch-recent-go" onClick={() => pickRecent(q)}>
                    <ClockIcon />
                    {q}
                  </button>
                  <button type="button" className="srch-recent-x" aria-label={`「${q}」を最近の検索から消す`} onClick={() => dropRecent(q)}>
                    <CloseIcon />
                  </button>
                </span>
              ))}
            </div>
          )}

          {titleError && <ErrorNotice onRetry={() => void runTitle(titleQuery, true)} />}

          {titleLoading && titleQuery.trim() && (
            <div className="srch-wait srch-fade" role="status" aria-live="polite">
              <span className="srch-spinner" aria-hidden="true" />
              <div className="srch-wait-text">
                <b>検索しています</b>
              </div>
            </div>
          )}

          {titleQuery.trim() && titleResult && (
            <>
              {titleResult.works.length > 0 && (
                <div className="srch-res-head srch-fade" key={`th-${titleRun}`}>
                  <div>
                    <div className="srch-eyebrow">作品名で探した結果</div>
                    <p className="srch-sentence">「{titleResult.query}」を含むタイトル</p>
                  </div>
                  <ResultCount count={titleResult.works.length} />
                </div>
              )}
              <ResultGrid
                key={`tg-${titleRun}`}
                works={titleResult.works}
                onOpenWork={onOpenWork}
                statusLabelFor={statusLabelFor}
                emptyText={`「${titleResult.query}」というタイトルの作品は見つかりません。内容で探すも試してください。`}
              />
            </>
          )}
        </section>
      ) : (
        <section className="srch-swap-in" key="content" aria-label="内容で探す">
          <form className="srch-content-card" role="search" onSubmit={onContentSubmit}>
            <label className="srch-sr-only" htmlFor="srch-content-input">
              探している作品の内容
            </label>
            <textarea
              id="srch-content-input"
              className="srch-content-input"
              rows={2}
              maxLength={CONTENT_MAX}
              enterKeyHint="search"
              placeholder="どんな作品を探していますか？ 例: 異世界で料理する漫画"
              value={contentText}
              onChange={(e) => setContentText(e.target.value)}
              onKeyDown={onContentKeyDown}
            />
            <div className="srch-content-foot">
              <span className="text-muted srch-count">
                {contentText.length} / {CONTENT_MAX}
              </span>
              <button type="submit" className="btn srch-btn-accent" disabled={!contentText.trim() || contentLoading}>
                探す
              </button>
            </div>
          </form>

          <div className="srch-chips">
            <span className="text-muted srch-chips-label">入力例</span>
            {CONTENT_EXAMPLES.map((ex) => (
              // 押すと欄に入るだけ。検索はしない（書き足してから「探す」）
              <button type="button" className="srch-example" key={ex} onClick={() => setContentText(ex)}>
                {ex}
              </button>
            ))}
          </div>

          {contentLoading && (
            <>
              <div className="srch-wait srch-fade" role="status" aria-live="polite">
                <span className="srch-spinner" aria-hidden="true" />
                <div className="srch-wait-text">
                  <b>AI が条件を考えています（{elapsed} 秒）</b>
                  <span className="text-muted">　数秒〜十数秒かかります</span>
                </div>
                <button type="button" className="btn btn-secondary" onClick={cancelContent}>
                  やめる
                </button>
              </div>
              <div className="poster-grid" aria-hidden="true">
                {Array.from({ length: 8 }, (_, i) => (
                  <div className="srch-fade" style={staggerStyle(i)} key={i}>
                    <div className="poster-thumb srch-skel" />
                    <div className="srch-skel srch-skel-line" />
                  </div>
                ))}
              </div>
            </>
          )}

          {contentError && !contentLoading && <ErrorNotice onRetry={() => void runContent(true)} />}

          {contentResult && !contentLoading && (
            <>
              {contentResult.fallback && <FallbackNotice reason={contentResult.fallback} onRetry={() => void runContent(true)} />}
              {/* 0 件のときも条件は出す。どう読み取られたかが分かれば文を直せる */}
              {contentResult.conditions && (
                <div className="srch-res-head srch-fade" key={`ch-${contentRun}`}>
                  <div>
                    <div className="srch-eyebrow">AI が読み取った条件</div>
                    <ConditionSentence conditions={contentResult.conditions} />
                  </div>
                  <ResultCount count={contentResult.works.length} />
                </div>
              )}
              <ResultGrid
                key={`cg-${contentRun}`}
                works={contentResult.works}
                onOpenWork={onOpenWork}
                statusLabelFor={statusLabelFor}
                emptyText="一致する作品が見つかりません。"
              />
            </>
          )}
        </section>
      )}
    </div>
  )
}

// ===== 部品 =====

function ResultCount({ count }: { count: number }) {
  return (
    <div className="srch-count-wrap" aria-live="polite">
      <span className="srch-count-big">{count}</span>
      <span className="text-muted">作品</span>
    </div>
  )
}

// ポスターは既存の PosterCard を使う（表紙が無いときの色ブロックと頭文字をほかの画面とそろえる）。
// フェードは外側の要素に付ける。PosterCard 自身に付けると、animation の transform が
// .poster-card:hover の持ち上がる動きを上書きしてしまう。
function ResultGrid({
  works,
  onOpenWork,
  statusLabelFor,
  emptyText,
}: {
  works: WorkSummary[]
  onOpenWork: (work: WorkSummary) => void
  statusLabelFor: (work: WorkSummary) => string | null
  emptyText: string
}) {
  if (works.length === 0) return <p className="text-muted srch-empty srch-fade">{emptyText}</p>
  return (
    <div className="poster-grid">
      {works.map((w, i) => {
        const status = statusLabelFor(w)
        return (
          <div className="srch-reveal" style={staggerStyle(i)} key={w.id}>
            <PosterCard
              item={{ title: w.title, coverImageUrl: w.coverUrl }}
              badgeLabel={status ? `${status} ✓` : null}
              caption={w.genres.slice(0, 2).join('・')}
              onClick={() => onOpenWork(w)}
            />
          </div>
        )
      })}
    </div>
  )
}

/** 条件を「漫画の、ファンタジーから、異世界と料理のタグがある作品」と文で読ませる */
function ConditionSentence({ conditions }: { conditions: AiConditions }) {
  const { kind, kindFromText, genres, tags, excludedGenres, excludedTags, query, character } = conditions
  const excluded = [...excludedGenres, ...excludedTags]
  return (
    <>
      <p className="srch-sentence">
        <span className="srch-tok srch-tok-kind">{KIND_LABEL[kind]}</span>の、
        {genres.length > 0 && (
          <>
            {genres.map((g, i) => (
              <span key={g}>
                <span className="srch-tok srch-tok-genre">{g}</span>
                {i < genres.length - 1 ? '・' : ''}
              </span>
            ))}
            から、
          </>
        )}
        {tags.length > 0 && (
          <>
            {tags.map((t, i) => (
              <span key={t}>
                <span className="srch-tok srch-tok-tag">{t}</span>
                {i < tags.length - 1 ? 'と' : ''}
              </span>
            ))}
            のタグがある
          </>
        )}
        {character && (
          <>
            {tags.length > 0 && '、'}
            <span className="srch-tok srch-tok-character">{character}</span>が出る
          </>
        )}
        作品
        {kindFromText && <span className="text-muted srch-kind-note">（種別は文から）</span>}
      </p>
      {/* 除外とキーワードは案8の文には無いが、今の画面ではチップで出していたので落とさずに残す */}
      {(query || excluded.length > 0) && (
        <p className="text-muted srch-cond-extra">
          {query && <>キーワード「{query}」</>}
          {query && excluded.length > 0 && '　'}
          {excluded.length > 0 && <>除外: {excluded.join('・')}</>}
        </p>
      )}
    </>
  )
}

/** 従来の作品名検索に落ちた理由を 3 つで出し分ける */
function FallbackNotice({ reason, onRetry }: { reason: FallbackReason; onRetry: () => void }) {
  if (reason === 'no_conditions') {
    return (
      <div className="srch-notice srch-notice-info srch-fade" role="status">
        <InfoIcon />
        <div>
          <div className="srch-notice-title">文から条件を読み取れなかったので、作品名で検索しました</div>
          <div className="srch-notice-body">ジャンル・舞台・雰囲気などを書くと、内容で探せます。上の入力例も試してください。</div>
        </div>
      </div>
    )
  }
  if (reason === 'ai_failed') {
    return (
      <div className="srch-notice srch-notice-warn srch-fade" role="status">
        <WarnIcon />
        <div>
          <div className="srch-notice-title">AI の処理に失敗したので、作品名で検索しました</div>
          <div className="srch-notice-body">入力をそのままキーワードにした結果です。</div>
          <div className="srch-notice-actions">
            <button type="button" className="btn btn-primary" onClick={onRetry}>
              もう一度 AI で探す
            </button>
          </div>
        </div>
      </div>
    )
  }
  return (
    <div className="srch-notice srch-notice-off srch-fade" role="status">
      <OffIcon />
      <div>
        <div className="srch-notice-title">いまは AI が使えないので、作品名で検索しました</div>
        <div className="srch-notice-body">内容で探すは、しばらくしてからお試しください。</div>
      </div>
    </div>
  )
}

/** 通信そのものが失敗したとき（従来検索に落ちた 3 つの理由とは別） */
function ErrorNotice({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="srch-notice srch-notice-warn srch-fade" role="alert">
      <WarnIcon />
      <div>
        <div className="srch-notice-title">検索できませんでした</div>
        <div className="srch-notice-body">通信を確かめて、もう一度お試しください。</div>
        <div className="srch-notice-actions">
          <button type="button" className="btn btn-primary" onClick={onRetry}>
            もう一度
          </button>
        </div>
      </div>
    </div>
  )
}

// ===== アイコン（線のアイコン。色は currentColor） =====

function SearchIcon({ size }: { size: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.2} strokeLinecap="round" aria-hidden="true">
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" />
    </svg>
  )
}
function ClockIcon() {
  return (
    <svg width={13} height={13} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </svg>
  )
}
function CloseIcon() {
  return (
    <svg width={10} height={10} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={3} strokeLinecap="round" aria-hidden="true">
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  )
}
function InfoIcon() {
  return (
    <svg width={18} height={18} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v5M12 8h.01" />
    </svg>
  )
}
function WarnIcon() {
  return (
    <svg width={18} height={18} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 3 2 20h20L12 3Z" />
      <path d="M12 10v4M12 17h.01" />
    </svg>
  )
}
function OffIcon() {
  return (
    <svg width={18} height={18} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <path d="M8 12h8" />
    </svg>
  )
}
function LockIcon() {
  return (
    <svg width={18} height={18} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden="true">
      <rect x="5" y="11" width="14" height="9" rx="2" />
      <path d="M8 11V8a4 4 0 0 1 8 0v3" />
    </svg>
  )
}
