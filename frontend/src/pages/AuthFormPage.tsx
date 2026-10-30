import { useEffect, useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { OnboardingModal } from '../components/OnboardingModal'
import { useAuth } from '../context/AuthContext'
import { resetKindToAnime } from '../hooks/useKind'
import { api } from '../lib/api'
import { hasSeenOnboarding } from '../lib/onboarding'
import { safeNextPath } from '../lib/safeNextPath'

const COPY = {
  login: {
    title: 'ログイン',
    lead: '自分の記録を見るにはログインしてください',
    submit: 'ログイン',
    switchText: 'アカウントをお持ちでない場合は',
    switchLabel: '新規登録',
    switchTo: '/signup',
    google: 'Google でログイン',
  },
  signup: {
    title: '新規登録',
    lead: 'メールアドレスとパスワードでアカウントを作成します',
    submit: '登録してはじめる',
    switchText: '既にアカウントをお持ちの場合は',
    switchLabel: 'ログイン',
    switchTo: '/login',
    google: 'Google で登録',
  },
} as const

// Google から戻ってきて失敗したとき、サーバーが付けてくる ?google_error= の理由（handler/google_auth_handler.go）
const GOOGLE_ERRORS: Record<string, string> = {
  cancelled: 'Google でのログインがキャンセルされました',
  state: 'ログインの有効期限が切れました。もう一度お試しください',
  unverified: 'Google アカウントのメールアドレスが確認されていないため、ログインできません',
  linked: 'このメールアドレスのアカウントには、別の Google アカウントが紐付いています',
  failed: 'Google でのログインに失敗しました。時間をおいてもう一度お試しください',
  unavailable: 'Google でのログインは現在使えません',
}

function googleErrorMessage(search: string): string | null {
  const reason = new URLSearchParams(search).get('google_error')
  if (!reason) return null
  return GOOGLE_ERRORS[reason] ?? GOOGLE_ERRORS.failed
}

export function AuthFormPage({ mode }: { mode: 'login' | 'signup' }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const { refresh } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [error, setError] = useState<string | null>(() => googleErrorMessage(location.search))
  const [submitting, setSubmitting] = useState(false)
  // 設定の無い環境（GOOGLE_CLIENT_ID が無い）ではボタンを出さない。取れなかったときも出さない側に倒す
  const [googleEnabled, setGoogleEnabled] = useState(false)
  const copy = COPY[mode]

  useEffect(() => {
    api
      .authProviders()
      .then((p) => setGoogleEnabled(p.google))
      .catch(() => setGoogleEnabled(false))
  }, [])

  // Google の同意画面を経由するので fetch ではなく画面遷移にする。戻ってきたらサーバーが next へ送る
  const next = safeNextPath(location.search)
  const googleHref = '/api/auth/google/start' + (next ? '?' + new URLSearchParams({ next }).toString() : '')
  // 初期値で 1 回だけ判定する。モーダルが表示時に「見た」を保存するので、描画のたびに読むと開いた直後に消える
  const [showOnboarding, setShowOnboarding] = useState(() => mode === 'login' && !hasSeenOnboarding())

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      if (mode === 'login') {
        await api.login({ email, password: password })
      } else {
        await api.signup({ email, password })
      }
      resetKindToAnime()
      await refresh()
      navigate(safeNextPath(location.search) || '/')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setSubmitting(false)
    }
  }

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 'var(--space-6)' }}>
      <div className="card elev-md" style={{ width: '100%', maxWidth: 360, padding: 'var(--space-6)', gap: 'var(--space-4)' }}>
        <div className="brand-badge" style={{ marginBottom: 'var(--space-2)', alignSelf: 'flex-start' }}>
          <svg width="21" height="21" viewBox="0 0 64 64" style={{ flex: 'none' }}>
            <path d="M32 18c-6-4-14-5-22-4v34c8-1 16 0 22 4V18Z" fill="var(--color-bg)" />
            <path d="M32 18c6-4 14-5 22-4v34c-8-1-16 0-22 4V18Z" fill="var(--color-bg)" opacity="0.5" />
            <path d="M27 27l13 7-13 7V27Z" fill="var(--color-accent)" />
          </svg>
          <span>みろく</span>
        </div>

        <h2 style={{ fontSize: 22, marginBottom: 2 }}>{copy.title}</h2>
        <p className="text-muted" style={{ fontSize: 13 }}>{copy.lead}</p>

        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div className="field">
            <label htmlFor="email">メールアドレス</label>
            <input
              className="input"
              id="email"
              type="email"
              required
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          <div className="field">
            <label htmlFor="password">{mode === 'signup' ? 'パスワード（8文字以上）' : 'パスワード'}</label>
            <input
              className="input"
              id="password"
              type="password"
              required
              minLength={mode === 'signup' ? 8 : undefined}
              autoComplete={mode === 'signup' ? 'new-password' : 'current-password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          {error && (
            <div className="text-muted" style={{ fontSize: 12, color: '#c0392b' }}>
              {error}
            </div>
          )}
          <button className="btn btn-primary" type="submit" disabled={submitting} style={{ justifyContent: 'center' }}>
            {copy.submit}
          </button>
        </form>

        {googleEnabled && (
          <>
            <div className="text-muted" style={{ fontSize: 12, textAlign: 'center' }}>
              または
            </div>
            <a
              className="btn btn-secondary"
              href={googleHref}
              // メールアドレスでのログインと同じく、ログイン直後はアニメから始める。
              // 戻ってくるのはサーバーのリダイレクトなので、離れる前に済ませておく
              onClick={resetKindToAnime}
              style={{ justifyContent: 'center', gap: 'var(--space-2)' }}
            >
              <GoogleLogo />
              {copy.google}
            </a>
          </>
        )}

        <p className="text-muted" style={{ fontSize: 13, textAlign: 'center', margin: 0 }}>
          {copy.switchText} <Link to={copy.switchTo}>{copy.switchLabel}</Link>
        </p>
      </div>

      {showOnboarding && (
        <OnboardingModal
          onClose={() => setShowOnboarding(false)}
          // /login と /signup は同じ位置の AuthFormPage なので、遷移してもコンポーネントは作り直されず
          // showOnboarding が残る。先に閉じないと、URL だけ変わってモーダルが新規登録画面を覆ったままになる。
          // ?next= は引き継ぎ、登録後に元々開こうとしていた画面へ戻れるようにする
          onStart={() => {
            setShowOnboarding(false)
            navigate({ pathname: '/signup', search: location.search })
          }}
        />
      )}
    </div>
  )
}

// Google のブランドガイドラインにある「G」のロゴ
function GoogleLogo() {
  return (
    <svg width="18" height="18" viewBox="0 0 48 48" aria-hidden="true" style={{ flex: 'none' }}>
      <path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z" />
      <path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z" />
      <path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z" />
      <path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z" />
    </svg>
  )
}
