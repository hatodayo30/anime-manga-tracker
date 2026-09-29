ALTER TABLE records
    DROP CONSTRAINT records_progress_unit_matches_media_type,
    DROP COLUMN progress_unit;
