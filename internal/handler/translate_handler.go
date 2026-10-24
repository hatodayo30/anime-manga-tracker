package handler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/translate"
)

type TranslateHandler struct {
	usecase *translate.Usecase
}

func NewTranslateHandler(u *translate.Usecase) *TranslateHandler {
	return &TranslateHandler{usecase: u}
}

type translateRequest struct {
	Text   string `json:"text"`
	Title  string `json:"title"`
	Target string `json:"target"`
}

// maxTranslateTextBytes は 1 リクエストで受け付ける text のバイト数上限。
//
// このエンドポイントは未認証で叩けるうえ、internal/translate の Client が text を
// 450 バイト（chunkByteLimit）ごとのチャンクに分けて MyMemory を 1 本ずつ叩く。
// つまり上限を設けないと「1 リクエスト = 上流 N 本」の増幅器になり、
// MyMemory の匿名利用は IP ごとの日次上限なので、ALB の IP 単位で弾かれて
// 翻訳機能そのものが死ぬ（BACKLOG 4-a）。
//
// そこで上流の本数から逆算する。チャンクは文単位で詰めるため、
// 1 文が 450 バイトの半分強（約 226 バイト）だと次の文が入らず、
// 1 チャンク約 226 バイトが最悪ケースになる。したがって上流の本数は
// おおよそ bytes/226 本。上限 4500 バイトなら最悪でも約 20 本、
// 通常の文章なら 10 本（4500/450）に収まる。
//
// AI 翻訳（Claude）でもこの上限は残す。こちらは上流の本数ではなく 1 回あたりのトークン量
// （＝費用）の上限として効く。
//
// 正当な入力は AniList の description（あらすじ）だけで、長いものでも
// 2000〜3000 バイト程度なので 4500 バイトなら切り捨てる心配はない。
// バイト数で測るのは、分割側が len() のバイト数で判定しているのに合わせるため。
const maxTranslateTextBytes = 450 * 10

// maxTranslateTitleBytes は title のバイト数上限。title は AI のプロンプトに入るだけで
// MyMemory には送らないので、text の上限とは別に、作品名として十分な長さで切る。
// AniList で最も長い部類の日本語タイトルでも 200 バイト前後。
const maxTranslateTitleBytes = 300

// Translate handles POST /api/translate { "text": "...", "title": "...", "target": "ja" }
// 作品モーダルのあらすじを表示言語（JA）に合わせて翻訳するために使う。
// title は作品の表示名で、AI 翻訳に作品名の表記を合わせさせるために使う（省略可）。
// あらすじは数百〜千文字を超えることがあるため、GETクエリではなくPOSTボディで受け取る。
// ルーターで OptionalUser を被せており、ログイン済みのときだけ AI 翻訳を使う（BACKLOG 9-a）。
func (h *TranslateHandler) Translate(c echo.Context) error {
	var in translateRequest
	if err := c.Bind(&in); err != nil {
		return writeError(c, http.StatusBadRequest, "invalid request body")
	}

	if len(in.Text) > maxTranslateTextBytes {
		return writeError(c, http.StatusBadRequest,
			fmt.Sprintf("text too long: %d bytes (max %d)", len(in.Text), maxTranslateTextBytes))
	}

	if len(in.Title) > maxTranslateTitleBytes {
		return writeError(c, http.StatusBadRequest,
			fmt.Sprintf("title too long: %d bytes (max %d)", len(in.Title), maxTranslateTitleBytes))
	}

	translated, err := h.usecase.Translate(c.Request().Context(), in.Text, in.Title, in.Target, middleware.UserFromContext(c) != nil)
	if err != nil {
		// 詳細は外部に出さずログにだけ残す。上流の URL や内部事情がクライアントに漏れるため（BACKLOG 4-d）。
		c.Logger().Errorf("translate: %v", err)
		return writeError(c, http.StatusBadGateway, "translate failed")
	}

	return c.JSON(http.StatusOK, map[string]string{"translated": translated})
}
