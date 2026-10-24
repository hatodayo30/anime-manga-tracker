-- AI（Claude）によるあらすじ翻訳の保存先。同じ原文は 2 回目以降 Claude を呼ばずにここから返す。
--
-- キーは anilist_id ではなく原文の SHA-256。/api/translate は原文をクライアントから受け取るので、
-- 作品 ID をキーにすると任意の文章をその作品の翻訳として保存させ、全利用者に配れてしまう。
-- 原文ハッシュなら、汚せるのは「同じ原文を送った人」の結果だけになる（BACKLOG 9-a）。
--
-- MyMemory の結果は保存しない。品質が低く、保存すると後から AI 翻訳に置き換わる機会が無くなるため。
CREATE TABLE translations (
    source_sha256  BYTEA       NOT NULL,
    target_lang    TEXT        NOT NULL,
    translated     TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_sha256, target_lang)
);
