import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { api } from '../lib/api'

// アカウント画面。スマホ幅ではサイドバーが隠れてログアウトにたどり着けなかったので、
// ログアウトもここに置き、スマホは上部のアイコンからここへ来る（Layout）。
export function AccountPage() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  return (
    <>
      <div style={{ marginBottom: 'var(--space-6)' }}>
        <h2 style={{ margin: 0 }}>アカウント設定</h2>
      </div>

      <section className="card elev-sm account-section">
        <h5 className="shelf-h">ログイン中のアカウント</h5>
        <div style={{ fontSize: 14, wordBreak: 'break-all' }}>{user?.email}</div>
        <button className="btn btn-secondary" style={{ alignSelf: 'flex-start', marginTop: 'var(--space-2)' }} onClick={handleLogout}>
          ログアウト
        </button>
      </section>

      {/* パスワードを持たない（Google だけで登録した）ユーザーに欄を出すと、送っても必ず失敗する。
          providers が無いとき（分からない）は従来どおり出す */}
      {user?.providers && !user.providers.includes('password') ? (
        <section className="card elev-sm account-section">
          <h5 className="shelf-h">パスワード</h5>
          <p className="text-muted" style={{ fontSize: 12, margin: 0 }}>
            Google アカウントでログインしているため、パスワードは設定されていません。
          </p>
        </section>
      ) : (
        <ChangePasswordSection />
      )}
    </>
  )
}

function ChangePasswordSection() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    setDone(false)
    // 確認欄はサーバーに送らない。打ち間違えたまま変えると、本人も新しいパスワードを知らない状態になる
    if (next !== confirm) {
      setError('新しいパスワードが確認用と一致しません')
      return
    }
    setSubmitting(true)
    try {
      await api.changePassword({ currentPassword: current, newPassword: next })
      setCurrent('')
      setNext('')
      setConfirm('')
      setDone(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section className="card elev-sm account-section">
      <h5 className="shelf-h">パスワードの変更</h5>
      <p className="text-muted" style={{ fontSize: 12, margin: 0 }}>
        変更すると、この端末以外ではログアウトされます。
      </p>
      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', marginTop: 'var(--space-2)' }}>
        <div className="field">
          <label htmlFor="current-password">現在のパスワード</label>
          <input
            className="input"
            id="current-password"
            type="password"
            required
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="new-password">新しいパスワード（8文字以上）</label>
          <input
            className="input"
            id="new-password"
            type="password"
            required
            minLength={8}
            autoComplete="new-password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="confirm-password">新しいパスワード（確認）</label>
          <input
            className="input"
            id="confirm-password"
            type="password"
            required
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </div>
        {error && (
          <div className="text-muted" style={{ fontSize: 12, color: '#c0392b' }}>
            {error}
          </div>
        )}
        {done && (
          <div role="status" style={{ fontSize: 12, color: 'var(--color-accent)' }}>
            パスワードを変更しました
          </div>
        )}
        <button className="btn btn-primary" type="submit" disabled={submitting} style={{ alignSelf: 'flex-start' }}>
          変更する
        </button>
      </form>
    </section>
  )
}
