import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

// 削除の依頼・問い合わせの窓口。公開ページに載るので、個人のアドレスではなくこのアプリ専用のものにしている。
const CONTACT_EMAIL = 'mirokuapp.contact@gmail.com'

// 制定日・改定日。内容を変えたら更新する
const UPDATED_ON = '2026年11月2日'

// プライバシーポリシー。Google の OAuth 同意画面を本番に公開するにはこの URL が必須（BACKLOG 8）。
// 書いてある内容はすべて実装に対応している。実装を変えて送る先・保存するものが変わったら、ここも直すこと:
// - AI に渡す記録の項目と件数: usecase/recommend の Taste と maxTasteRecords / maxMemoRunes
// - Google から受け取る項目: internal/google の Scopes（openid / email）
// - セッションの期限: usecase/auth の SessionTTL
export function PrivacyPage() {
  return (
    <div style={{ minHeight: '100vh', display: 'flex', justifyContent: 'center', alignItems: 'flex-start', padding: 'var(--space-6)' }}>
      <article
        className="card elev-md"
        style={{ width: '100%', maxWidth: 720, padding: 'var(--space-6)', gap: 'var(--space-4)', lineHeight: 1.8 }}
      >
        <h1 style={{ fontSize: 24, margin: 0 }}>プライバシーポリシー</h1>
        <p className="text-muted" style={{ fontSize: 13, margin: 0 }}>
          最終更新日: {UPDATED_ON}
        </p>
        <p style={{ margin: 0 }}>
          「みろく」（以下「本サービス」）は、アニメと漫画の視聴・読書の記録を付けるための個人開発のサービスです。
          本サービスが取得する情報と、その扱いを以下に定めます。
        </p>

        <Section title="1. 取得する情報">
          <ul>
            <li>
              <strong>メールアドレス</strong> — 登録とログインに使います。
            </li>
            <li>
              <strong>パスワード</strong> — 元のパスワードは保存せず、復元できない形（bcrypt によるハッシュ）にしたものだけを保存します。
            </li>
            <li>
              <strong>Google アカウントの情報</strong>（Google でログインした場合）—
              Google アカウントを識別する ID とメールアドレスだけを受け取ります。氏名・プロフィール画像・連絡先などは受け取りません。
            </li>
            <li>
              <strong>記録の内容</strong> — 登録した作品、状態（見たい・見てる・見た）、進み具合、評価、メモ。
            </li>
            <li>
              <strong>Cookie</strong> — ログイン状態を保つための Cookie（有効期限 30 日）と、Google でログインする間だけ使う一時的な
              Cookie（10 分）。アクセス解析や広告のための Cookie は使っていません。
            </li>
          </ul>
        </Section>

        <Section title="2. 利用目的">
          <p>取得した情報は、次の目的にだけ使います。</p>
          <ul>
            <li>ログインと、本人の確認</li>
            <li>記録の保存と表示</li>
            <li>記録にもとづくおすすめの作品の提示と、作品の検索</li>
          </ul>
        </Section>

        <Section title="3. 外部のサービスへの送信">
          <p>本サービスは、機能を提供するために次の外部サービスへ情報を送ります。</p>
          <ul>
            <li>
              <strong>Anthropic（AI）</strong> — AI によるおすすめを使うと、最近更新した記録のうち最大 50 件について、作品名・状態・評価・ジャンル・メモ（1 件あたり先頭 200 文字まで）を送ります。
              文章で作品を探す機能を使うと、入力した文を送ります。どちらも、メールアドレスなどアカウントを特定できる情報は送りません。
              あらすじの翻訳のために、作品のあらすじ（公開されている情報）を送ることもあります。
            </li>
            <li>
              <strong>Google</strong> — Google でログインするときに、認証のためのやりとりを行います。
            </li>
            <li>
              <strong>作品情報・翻訳のサービス</strong>（AniList、Jikan、MyMemory、楽天ブックス）— 作品の検索語や作品の ID、あらすじを送り、作品の情報や翻訳を受け取ります。
              利用者のアカウントの情報や記録は送りません。
            </li>
            <li>
              <strong>Amazon Web Services</strong> — 本サービスのサーバーとデータベースは AWS（東京リージョン）にあり、取得した情報はそこに保存されます。
            </li>
          </ul>
          <p>上記のほかに、取得した情報を第三者に提供・販売することはありません（法令にもとづく場合を除きます）。</p>
        </Section>

        <Section title="4. 外部サイトへのリンク">
          <p>
            新刊の一覧などに、楽天ブックスの商品ページへのリンク（アフィリエイトリンク）があります。リンク先での情報の扱いは、楽天グループのプライバシーポリシーに従います。
          </p>
        </Section>

        <Section title="5. 保存期間と削除">
          <p>
            アカウントの情報と記録は、アカウントがある間保存します。ログイン状態の情報は有効期限が切れると削除します。
          </p>
          <p>
            アカウントと記録の削除を希望する場合は、登録したメールアドレスを添えて下記の連絡先までご連絡ください。確認のうえ、すべて削除します。
          </p>
        </Section>

        <Section title="6. このポリシーの変更">
          <p>内容を変更する場合は、このページで告知します。</p>
        </Section>

        <Section title="7. 連絡先">
          <p>
            <a href={`mailto:${CONTACT_EMAIL}`}>{CONTACT_EMAIL}</a>
          </p>
        </Section>

        <p style={{ margin: 0, fontSize: 13 }}>
          <Link to="/login">ログイン画面へ戻る</Link>
        </p>
      </article>
    </div>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h2 style={{ fontSize: 17, margin: '0 0 var(--space-2)' }}>{title}</h2>
      {children}
    </section>
  )
}
