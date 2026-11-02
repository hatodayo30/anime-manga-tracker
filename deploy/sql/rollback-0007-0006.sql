-- 0007 → 0006 の順に巻き戻し、0005 までのイメージ（75d2102 以前）が動くスキーマに戻す。
-- 中身は migrations/0007_user_identities.down.sql と migrations/0006_hash_session_tokens.down.sql をそのまま並べ、
-- 最後に applied_migrations の記録を消したもの。適用済みのマイグレーションは書き換えないので、二重管理でもずれない。
--
-- deploy/run-sql.sh で流す（--single-transaction なので、どこかで失敗すれば全体が巻き戻る。BEGIN / COMMIT は書かない）。
-- 0007 が未適用の DB に流すと、最初の ADD COLUMN が「既にある」で失敗して何も変わらない（2 回流しても安全）。
--
-- 失うもの: すべてのセッション（全員ログインし直し）と Google の紐付け。パスワードのハッシュと記録は失わない。
-- Google だけで登録したユーザーはパスワードが空文字になり、古いイメージではログインできない（記録は残る）。
-- 手順と、なぜ古いイメージに戻す前にこれが要るかは docs/DEPLOY-AWS.md の「マイグレーションを含むデプロイを戻す」。

-- ===== migrations/0007_user_identities.down.sql =====
ALTER TABLE users ADD COLUMN password_hash TEXT;

UPDATE users u SET password_hash = i.password_hash
FROM user_identities i
WHERE i.user_id = u.id AND i.provider = 'password';

UPDATE users SET password_hash = '' WHERE password_hash IS NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;

DROP TABLE user_identities;

-- ===== migrations/0006_hash_session_tokens.down.sql =====
DELETE FROM sessions;

ALTER TABLE sessions RENAME COLUMN token_hash TO token;

-- 消さないと、新しいイメージを再デプロイしたときにランナーが 0006 / 0007 を適用済みとみなして流さず、
-- 新しいコードが古いスキーマの上で動いてしまう
DELETE FROM applied_migrations WHERE version IN (6, 7);

-- 結果の確認（ログに出る）
SELECT version, name FROM applied_migrations ORDER BY version;
SELECT count(*) AS users, count(*) FILTER (WHERE password_hash = '') AS google_only_users FROM users;
