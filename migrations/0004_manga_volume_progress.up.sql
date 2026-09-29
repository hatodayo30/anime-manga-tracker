-- 漫画の進捗を話数（chapters）ベースから巻数（volumes）ベースに切り替える。
-- records.progress / total は media_type で単位が決まる（アニメ=話数、漫画=巻数）ため
-- カラム追加はせず、単位が変わった漫画レコードの値をリセットする。
-- 既存の total は話数なので巻数に換算できない。NULL に戻したうえで、作品モーダルが
-- AniList の最新情報を取得したタイミングで巻数へ同期される（PATCH /api/records/:id の total）。
UPDATE records
   SET progress = 0,
       total    = NULL
 WHERE media_type = 'manga';
