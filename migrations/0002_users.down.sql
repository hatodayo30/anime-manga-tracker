ALTER TABLE records
    DROP CONSTRAINT records_user_anilist_media_key,
    ADD CONSTRAINT records_anilist_id_media_type_key UNIQUE (anilist_id, media_type),
    DROP COLUMN user_id;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
