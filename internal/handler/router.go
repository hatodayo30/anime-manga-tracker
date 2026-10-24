package handler

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Handlers はルーティングに必要なハンドラ・ミドルウェア一式をまとめたもの。
type Handlers struct {
	Record       *RecordHandler
	Auth         *AuthHandler
	Search       *SearchHandler
	Home         *HomeHandler
	Translate    *TranslateHandler
	Health       *HealthHandler
	RequireUser  echo.MiddlewareFunc
	OptionalUser echo.MiddlewareFunc
	WebDir       string

	// TrustProxy はレート制限のキーにするクライアント IP を X-Forwarded-For から取るかどうか。
	// 判断の理由は config.Config.TrustProxy を参照。
	TrustProxy bool
}

// レート制限の値。キーはクライアント IP（NewRouter の IPExtractor を参照）。
//
// ExpiresIn は「バケットが満タンに戻るまでの時間」以上にしておく。短いと、しばらく黙っていた
// 訪問者のバケットが満タンに戻る前に削除され、作り直しで満タンから再開できてしまう。
// どちらもその条件を満たすので、エントリの寿命は echo の既定（3 分）と同じにしている。
var (
	// 未認証で叩ける /api/*。正当な利用で最も密になるのは、ページを開いた瞬間の数本
	// （ホームなら me + 3 本、作品モーダルなら relations + translate）と、500ms デバウンスの検索入力
	// （最大 2 本/秒）。バーストでページ遷移の連打を吸収し、持続 2 本/秒で検索入力の上限を通す。
	// CGNAT や社内 NAT で複数人が 1 つの IP を共有しうるので、1 人分ぎりぎりには絞らない。
	//
	// これで守れるのは 1 つの IP からの洪水（ハンドラの goroutine、MyMemory への翻訳の増幅）まで。
	// 1 IP で 120 本/分は AniList のプロセス全体のバケット（24 本/分）より多いので、
	// AniList の枠を他の利用者から守っているのはこの制限ではなく、キャッシュと共有バケットの方。
	publicAPIRateLimit = middleware.RateLimiterMemoryStoreConfig{
		Rate:      2,
		Burst:     30,
		ExpiresIn: 3 * time.Minute, // 満タンまで 15 秒
	}

	// /api/auth/login。bcrypt cost 10 は 1 回あたり数十 ms のコア時間で、0.25 vCPU のタスクでは
	// 毎秒数回のログインで CPU が埋まる。つまりブルートフォースであると同時に CPU の DoS になる。
	// 人間の打ち間違いは数回で収まるので、1 IP あたり 5 回まで即時、以後 12 秒に 1 回。
	// 1 IP が使える bcrypt は毎分 5 回程度に収まり、総当たりは 1 IP で 1 日 7,200 回が上限になる。
	// 多数の IP からの分散攻撃は IP 単位の制限では止められない（BACKLOG 4-b のサインアップを閉じる、
	// アカウント単位のロックなどは別の話）。
	loginRateLimit = middleware.RateLimiterMemoryStoreConfig{
		Rate:      5.0 / 60,
		Burst:     5,
		ExpiresIn: 3 * time.Minute, // 満タンまで 60 秒
	}
)

// NewRouter はEchoのルーティングを構築する。静的ファイル配信のみ、既存の
// net/http.Handler実装（NewStaticHandler）をecho.WrapHandlerで接続する。
func NewRouter(h Handlers) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	// レート制限のキーにするクライアント IP の取り方。IPExtractor を nil のままにすると
	// echo は「XFF の左端 → X-Real-IP → RemoteAddr」の旧挙動に落ち、クライアントが書いた XFF を
	// そのまま信じる。なので false のときも ExtractIPDirect を明示する。
	//   - false（Phase 1、直接アクセス）: RemoteAddr が本物の接続元。XFF を信じるとヘッダを
	//     書き換えるだけでレート制限を自由に回避できる。
	//   - true（Phase 2、ALB 経由）: タスク SG が ALB SG からしか通さないので RemoteAddr は ALB。
	//     ExtractIPFromXFFHeader は右（サーバー側）から辿って、プライベート／ループバック／
	//     リンクローカル以外の最初の IP を返す。ALB の IP（VPC 内のプライベート）を飛ばし、
	//     ALB が右端に追記した接続元に当たる。クライアントが自分で書いた XFF はそれより左にあるので届かない。
	// **TRUST_PROXY=true の経路は実 ALB で未検証。** 前提は ALB の X-Forwarded-For 処理が既定の
	// append であること、ALB から見た接続元がパブリック IP であること（プライベート IP の
	// 接続元だと信頼済みとして飛ばされ、クライアントが書いた値まで辿ってしまう）。
	// ALB を入れたら、XFF を偽装したリクエストで 429 のキーが変わらないことを一度確認すること。
	if h.TrustProxy {
		e.IPExtractor = echo.ExtractIPFromXFFHeader()
	} else {
		e.IPExtractor = echo.ExtractIPDirect()
	}

	// X-Content-Type-Options: nosniff / X-Frame-Options: SAMEORIGIN / X-XSS-Protection を付ける。
	// 既定設定のままなので次の 2 つは出ない:
	//   - HSTS: HTTPS 化（Phase 2）までは意味がなく、max-age はブラウザに残り続けて取り消しが
	//     効かないので、HTTPS で運用が固まってから入れる。
	//   - Referrer-Policy: 現行ブラウザの既定（strict-origin-when-cross-origin）で足りる。
	// BodyLimit より先に置き、413 などのエラー応答にもヘッダが付くようにする。
	e.Use(middleware.Secure())

	// 未認証で叩ける POST（/api/translate・/api/auth/*）が上流 API への増幅器になるため、
	// ボディサイズに上限を置く。64K の根拠は「実際に必要な最大 × 十分な余裕」:
	//   - 最大のボディは /api/translate のあらすじ本文。AniList の description は長くても
	//     数 KB（UTF-8）で、これが実利用の上限。
	//   - 次に大きいのが /api/records の memo と genres で、いずれも桁が 1 つ小さい。
	// つまり 64K は正当な利用を一切壊さずに、1MB・10MB のボディを入口で落とせる値。
	//
	// ただしこれは「上限」であって対策そのものではない。64K でも translate は
	// chunkByteLimit=450 で分割されるので MyMemory 約 145 本に化ける（1MB なら約 2,300 本）。
	// 桁を 1 つ落とすだけの措置で、BACKLOG 4-a-1 の text 長の上限とは別に必要。
	// 同様に、GET のクエリ文字列（/api/anilist/media?ids=）にはこのミドルウェアは効かない。
	e.Use(middleware.BodyLimit("64K"))

	// RateLimiterMemoryStore のエントリが際限なく増えないか（直前に上限なしキャッシュを 2 つ潰したので確認した。
	// echo v4.15.4 の実装を読んだ結論）:
	//   - Allow のたびに「前回の掃除から ExpiresIn 以上経っていれば、ExpiresIn 以上来ていない
	//     訪問者を全削除」する。なのでエントリ数は「直近 ExpiresIn の 2 倍程度の間に来た異なる IP の数」で抑えられる。
	//     件数の上限（LRU のような打ち切り）は無い。
	//   - その窓の中で新しい IP を 1 つ作るには、TCP のハンドシェイクを終えたリクエストが 1 本要る
	//     （TRUST_PROXY=false なら RemoteAddr、true なら ALB が書いた値で、どちらも偽装できない）。
	//     IPv4 で持てるアドレスの数が実質の上限になり、1 エントリ数百バイトなので問題にならない。
	//   - 穴は 2 つ。IPv6 で /64 を持つ相手はアドレスを使い捨てられるので、窓の中のリクエスト数
	//     そのものが上限になる（今は IPv4 のみの構成なので未対処。IPv6 を有効にするなら /64 単位にまとめる）。
	//     もう 1 つは TRUST_PROXY=true を ALB の無い構成で使った場合で、XFF の値が自由に書けるため同じことが起きる。
	//   - リクエストが来なければ掃除も走らないが、そのときは増えもしないので問題ない。
	publicAPILimit := middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(publicAPIRateLimit))
	loginLimit := middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(loginRateLimit))

	// コンテナ/ALBのヘルスチェック用。静的ファイルのワイルドカードより先に登録する。
	e.GET("/healthz", h.Health.Healthz)

	records := e.Group("/api/records", h.RequireUser)
	records.GET("", h.Record.List)
	records.POST("", h.Record.Create)
	records.PATCH("/:id", h.Record.Update)
	records.DELETE("/:id", h.Record.Delete)

	// 未認証で叩ける /api/* にだけ IP 単位の制限をかける。/api/records は RequireUser の内側で
	// 利用者が特定できるので対象外。e.Group("/api") に Use すると /api/* の未定義パスを
	// Group が 404 で拾ってしまい静的ファイル配信の挙動が変わるので、ルートごとに渡している。
	e.GET("/api/search", h.Search.Search, publicAPILimit)
	e.GET("/api/anilist/media", h.Search.ByIDs, publicAPILimit)
	e.GET("/api/anilist/relations", h.Search.Relations, publicAPILimit)
	e.GET("/api/recommendations", h.Search.Recommendations, publicAPILimit)
	e.GET("/api/home/season-anime", h.Home.SeasonAnime, publicAPILimit)
	e.GET("/api/home/trending", h.Home.Trending, publicAPILimit)
	e.GET("/api/home/trending-manga", h.Home.TrendingManga, publicAPILimit)
	// translate は未ログインでも使えるが、ログイン済みなら AI 翻訳を使うので利用者を解決しておく
	e.POST("/api/translate", h.Translate.Translate, publicAPILimit, h.OptionalUser)

	// login は全体の制限に加えて専用の厳しい制限を重ねる（全体の 1 本としても数える）。
	// signup も bcrypt を回すが、BACKLOG 4-b で閉じる前提なので全体の制限だけにしている。
	e.POST("/api/auth/signup", h.Auth.SignUp, publicAPILimit)
	e.POST("/api/auth/login", h.Auth.Login, publicAPILimit, loginLimit)
	e.POST("/api/auth/logout", h.Auth.Logout, publicAPILimit)
	e.GET("/api/auth/me", h.Auth.Me, publicAPILimit)

	e.Any("/*", echo.WrapHandler(NewStaticHandler(h.WebDir)))

	return e
}
