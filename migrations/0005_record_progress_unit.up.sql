-- 進捗の記録単位を作品ごとに持たせる。
-- 漫画は紙の単行本で追う人と電子で話数を追う人がいて、同じ人でも作品によって読み方が
-- 変わるため、アプリ全体で単位を固定せずレコード単位で選べるようにする。
-- アニメは常に話数（episode）で、漫画だけが話数（chapter）と巻数（volume）を切り替えられる。
ALTER TABLE records
    ADD COLUMN progress_unit TEXT NOT NULL DEFAULT 'episode',
    ADD CONSTRAINT records_progress_unit_matches_media_type CHECK (
        (media_type = 'anime' AND progress_unit = 'episode')
        OR (media_type = 'manga' AND progress_unit IN ('chapter', 'volume'))
    ) NOT VALID;

-- 既存の漫画は既定の話数ベースに寄せる（CHECK制約を満たすため、検証より先に更新する）。
UPDATE records SET progress_unit = 'chapter' WHERE media_type = 'manga';

ALTER TABLE records VALIDATE CONSTRAINT records_progress_unit_matches_media_type;
