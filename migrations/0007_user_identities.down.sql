ALTER TABLE users ADD COLUMN password_hash TEXT;

UPDATE users u SET password_hash = i.password_hash
FROM user_identities i
WHERE i.user_id = u.id AND i.provider = 'password';

-- Google だけで登録したユーザーはパスワードを持たない。消すと記録ごと失われるので残し、空文字を入れて
-- NOT NULL に戻す。戻した後のコードでは空のハッシュとの比較が必ず失敗するので、ログインできないだけで済む
UPDATE users SET password_hash = '' WHERE password_hash IS NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;

DROP TABLE user_identities;
