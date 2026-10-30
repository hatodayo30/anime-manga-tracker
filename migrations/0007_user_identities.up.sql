-- ログインの手段（パスワード / Google）を users から分ける（BACKLOG 8-b の案 A）。
-- 「このユーザーはどの手段でログインできるか」が 1 箇所に出て、手段を足すときも行を足すだけで済む。
-- users.password_hash を NULL 許容にして google_sub を足す案（案 B）は、手段が増えるたびにカラムが増え、
-- パスワードの無いユーザーの空のハッシュが bcrypt の比較まで流れる経路ができるので採らなかった。
CREATE TABLE user_identities (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- 'password' / 'google'
    provider       TEXT NOT NULL,
    -- 手段の中で利用者を指す安定した識別子。Google は userinfo の sub（メールアドレスは変わりうるので使わない）。
    -- パスワードには外部の識別子が無いので users.id を文字列にして入れる。メールアドレスにしないのは、
    -- users.email と二重に持つと片方だけ変わる経路ができるため
    subject        TEXT NOT NULL,
    -- provider = 'password' の行だけが持つ
    password_hash  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, subject),
    -- 1 人が同じ手段を 2 つ持たない（別の Google アカウントを 2 つ紐付けない）
    UNIQUE (user_id, provider),
    CHECK ((provider = 'password') = (password_hash IS NOT NULL))
);

-- 既存のユーザーは全員パスワードで登録しているので、そのまま 'password' の行に移す
INSERT INTO user_identities (user_id, provider, subject, password_hash)
SELECT id, 'password', id::text, password_hash FROM users;

ALTER TABLE users DROP COLUMN password_hash;
