-- ハッシュから平文には戻せないので、セッションはすべて消す（利用者は全員ログインし直しになる）。
DELETE FROM sessions;

ALTER TABLE sessions RENAME COLUMN token_hash TO token;
