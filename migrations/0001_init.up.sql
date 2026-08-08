CREATE TABLE records (
    id               BIGSERIAL PRIMARY KEY,
    anilist_id       BIGINT NOT NULL,
    media_type       TEXT NOT NULL CHECK (media_type IN ('anime', 'manga')),
    title            TEXT NOT NULL,
    cover_image_url  TEXT NOT NULL DEFAULT '',
    genres           TEXT[] NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL CHECK (status IN ('want', 'active', 'done')),
    progress         INTEGER NOT NULL DEFAULT 0,
    total            INTEGER,
    next_airing_at   TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (anilist_id, media_type)
);

CREATE INDEX idx_records_type_status ON records (media_type, status);
