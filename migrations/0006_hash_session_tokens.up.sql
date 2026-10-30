-- セッショントークンを平文ではなく SHA-256 のハッシュ（16 進）で持つ（BACKLOG 4-d）。
-- DB の中身が漏れても、そこから Cookie を作ってなりすますことはできなくなる。
--
-- 既存のセッションは消さずにその場でハッシュに置き換える。Cookie に入っている平文は変わらず、
-- アプリ側が同じ計算（sha256 → 16 進）で引くので、利用者は誰もログアウトされない。
-- sha256() は PostgreSQL 11 以降の組み込みで、pgcrypto は要らない。
UPDATE sessions
   SET token = encode(sha256(convert_to(token, 'UTF8')), 'hex');

-- 中身が変わったことを列名で示す。平文で引くコードが残っていれば、列名の違いで落ちて気付ける。
ALTER TABLE sessions RENAME COLUMN token TO token_hash;
