import { useTitleLang } from '../context/TitleLangContext'

export function TitleLangToggle() {
  const { lang, setLang } = useTitleLang()
  return (
    <div className="seg" role="group" aria-label="作品名の表示言語" style={{ flex: 'none', marginBottom: 0 }}>
      {(['ja', 'en'] as const).map((l) => (
        <button
          key={l}
          type="button"
          className={`seg-opt${lang === l ? ' checked' : ''}`}
          aria-pressed={lang === l}
          onClick={() => setLang(l)}
        >
          {l === 'ja' ? 'JA' : 'EN'}
        </button>
      ))}
    </div>
  )
}
