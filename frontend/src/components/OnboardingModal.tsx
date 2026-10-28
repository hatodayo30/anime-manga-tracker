import { useEffect, useState, type ReactNode } from 'react'
import { markOnboardingSeen } from '../lib/onboarding'

const iconProps = { width: 40, height: 40, viewBox: '0 0 256 256', fill: 'none', stroke: 'currentColor', strokeWidth: 16 } as const
const smallIconProps = { ...iconProps, width: 17, height: 17 }

interface Step {
  icon: ReactNode
  title: string
  body: string
  note?: string
  features?: { icon: ReactNode; label: string }[]
}

const STEPS: Step[] = [
  {
    icon: (
      <svg width="44" height="44" viewBox="0 0 64 64">
        <path d="M32 18c-6-4-14-5-22-4v34c8-1 16 0 22 4V18Z" fill="var(--color-bg)" />
        <path d="M32 18c6-4 14-5 22-4v34c-8-1-16 0-22 4V18Z" fill="var(--color-bg)" opacity="0.5" />
        <path d="M27 27l13 7-13 7V27Z" fill="var(--color-accent)" />
      </svg>
    ),
    title: 'みろくへようこそ',
    body: 'アニメも漫画も、ひとつの記録に。見た作品・読んだ作品をまとめて残せる記録アプリです。',
    note: 'メールアドレスだけで、無料ではじめられます',
  },
  {
    icon: (
      <svg {...iconProps}>
        <rect x="44" y="44" width="168" height="168" rx="8" />
        <path d="M88 132l28 28 52-60" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    ),
    title: '進み具合を記録',
    body: '「見たい・見てる・見た」の状態と、アニメは話数・漫画は巻数で進み具合を残せます。評価やメモも付けられます。',
  },
  {
    icon: (
      <svg {...iconProps}>
        <circle cx="112" cy="112" r="76" />
        <line x1="168" y1="168" x2="220" y2="220" strokeLinecap="round" />
      </svg>
    ),
    title: '内容で作品を探す',
    body: '作品名がわからなくても大丈夫。「異世界で料理する漫画」のように、内容を文章で書いて探せます。',
  },
  {
    icon: (
      <svg {...iconProps}>
        <path d="M128 40l24 56 60 5-46 40 14 59-52-32-52 32 14-59-46-40 60-5Z" strokeLinejoin="round" />
      </svg>
    ),
    title: '便利な機能',
    body: 'ほかにもこんな機能があります。',
    features: [
      {
        icon: (
          <svg {...smallIconProps}>
            <rect x="40" y="48" width="176" height="152" rx="8" />
            <line x1="40" y1="92" x2="216" y2="92" />
            <line x1="84" y1="32" x2="84" y2="64" />
            <line x1="172" y1="32" x2="172" y2="64" />
          </svg>
        ),
        label: 'シーズンごとの放送作品を見る',
      },
      {
        icon: (
          <svg {...smallIconProps}>
            <path d="M128 32v32M128 192v32M32 128h32M192 128h32M60 60l23 23M173 173l23 23M60 196l23-23M173 83l23-23" strokeLinecap="round" />
            <circle cx="128" cy="128" r="36" />
          </svg>
        ),
        label: '漫画の新刊・新連載をチェック',
      },
      {
        icon: (
          <svg {...smallIconProps}>
            <path d="M128 40l24 56 60 5-46 40 14 59-52-32-52 32 14-59-46-40 60-5Z" strokeLinejoin="round" />
          </svg>
        ),
        label: '記録からあなたへのおすすめ',
      },
    ],
  },
]

export function OnboardingModal({ onClose, onStart }: { onClose: () => void; onStart: () => void }) {
  const [step, setStep] = useState(0)
  const current = STEPS[step]
  const isLast = step === STEPS.length - 1

  useEffect(() => {
    markOnboardingSeen()
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="card elev-lg onboarding"
        role="dialog"
        aria-modal="true"
        aria-labelledby="onboarding-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="onboarding-top">
          <button type="button" className="link-btn onboarding-skip" onClick={onClose}>
            スキップ
          </button>
          <button type="button" className="btn btn-icon" aria-label="閉じる" onClick={onClose}>
            <svg width="16" height="16" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="20" strokeLinecap="round">
              <line x1="64" y1="64" x2="192" y2="192" />
              <line x1="192" y1="64" x2="64" y2="192" />
            </svg>
          </button>
        </div>

        <div className="onboarding-dots" aria-label={`${STEPS.length} ページ中 ${step + 1} ページ目`}>
          {STEPS.map((_, i) => (
            <span key={i} className={i === step ? 'on' : i < step ? 'done' : undefined} />
          ))}
        </div>

        <div className="onboarding-body">
          <div className={step === 0 ? 'onboarding-icon brand' : 'onboarding-icon'}>{current.icon}</div>
          <h3 id="onboarding-title">{current.title}</h3>
          <p className="text-muted">{current.body}</p>
          {current.note && <div className="onboarding-note">{current.note}</div>}
          {current.features && (
            <ul className="onboarding-features">
              {current.features.map((f) => (
                <li key={f.label}>
                  {f.icon}
                  {f.label}
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="onboarding-actions">
          {step > 0 && (
            <button type="button" className="btn btn-secondary" onClick={() => setStep(step - 1)}>
              ‹ 戻る
            </button>
          )}
          {isLast ? (
            <button type="button" className="btn onboarding-cta" onClick={onStart}>
              はじめる
            </button>
          ) : (
            <button type="button" className="btn onboarding-cta" onClick={() => setStep(step + 1)}>
              次へ ›
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
