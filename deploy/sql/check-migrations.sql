-- 読み取りだけ。RDS に繋がることと、今どこまでマイグレーションが適用されているかを見る。
-- デプロイ前（0005 まで）でも後（0007 まで）でもエラーにならないよう、存在しないかもしれない表は直接引かない
SELECT version, name, applied_at FROM applied_migrations ORDER BY version;

-- 0006 が適用済みなら sessions.token_hash、未適用なら sessions.token が出る。
-- 0007 が適用済みなら users.password_hash は出ず、user_identities の列が出る
SELECT table_name, column_name
  FROM information_schema.columns
 WHERE table_schema = 'public'
   AND (   (table_name = 'sessions' AND column_name IN ('token', 'token_hash'))
        OR (table_name = 'users' AND column_name = 'password_hash')
        OR table_name = 'user_identities')
 ORDER BY table_name, column_name;

SELECT count(*) AS users FROM users;
