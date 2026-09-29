-- 0004 で失われた漫画の話数ベースの進捗は復元できない（巻数から話数への換算が不可能なため）。
-- ロールバック時は進捗をリセットしたままにする。
UPDATE records
   SET progress = 0,
       total    = NULL
 WHERE media_type = 'manga';
