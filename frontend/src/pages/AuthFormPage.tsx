import { useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { api } from '../lib/api'
import { safeNextPath } from '../lib/safeNextPath'

const COPY = {
  login: {
    title: 'ログイン',
    lead: '自分の記録を見るにはログインしてください',
    submit: 'ログイン',
    switchText: 'アカウントをお持ちでない場合は',
    switchLabel: '新規登録',
    switchTo: '/signup',
  },
  signup: {
    title: '新規登録',
    lead: 'メールアドレスとパスワードでアカウントを作成します',
    submit: '登録してはじめる',
    switchText: '既にアカウントをお持ちの場合は',
    switchLabel: 'ログイン',
    switchTo: '/login',
  },
} as const

export function AuthFormPage({ mode }: { mode: 'login' | 'signup' }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const { refresh } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const copy = COPY[mode]

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

        <p className="text-muted" style={{ fontSize: 13, textAlign: 'center', margin: 0 }}>
          {copy.switchText} <Link to={copy.switchTo}>{copy.switchLabel}</Link>
        </p>
      </div>
    </div>
  )
}
