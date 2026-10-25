package volumes_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/rakuten"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/volumes"
)

var jst = time.FixedZone("JST", 9*60*60)

// 2026-10-25 10:00 JST
func now() time.Time { return time.Date(2026, 10, 25, 10, 0, 0, 0, jst) }

// fakeGateway は pages の順にページを返し、呼ばれた題名とページを記録する。
type fakeGateway struct {
	pages [][]rakuten.Book
	err   error

	calls     int
	gotTitles []string
}

func (f *fakeGateway) ComicsByTitle(_ context.Context, title string, page int) (*rakuten.Page, error) {
	f.calls++
	f.gotTitles = append(f.gotTitles, title)
	if f.err != nil {
		return nil, f.err
	}
	if page > len(f.pages) {
		return &rakuten.Page{PageCount: len(f.pages), Items: []rakuten.Book{}}, nil
	}
	return &rakuten.Page{PageCount: len(f.pages), Items: f.pages[page-1]}, nil
}

func b(title, salesDate string) rakuten.Book { return rakuten.Book{Title: title, SalesDate: salesDate} }

// 題名の直後に巻の番号が続く本だけを数え、発売済みの番号の最大を既刊にすることを保証する（2026-10-25 の実データの形）。
func TestLatest_CountsReleasedVolumes(t *testing.T) {
	gw := &fakeGateway{pages: [][]rakuten.Book{{
		b("となりの君は、愛ばっか考えてる 3", "2027年02月25日"), // 予約は数えない
		b("となりの君は、愛ばっか考えてる 2", "2026年10月23日"),
		b("となりの君は、愛ばっか考えてる 1", "2026年03月24日"),
	}}}
	u := volumes.NewUsecase(gw, now)

	got, err := u.Latest(context.Background(), "となりの君は、愛ばっか考えてる")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Found || got.LatestVolume != 2 || got.SalesDate != "2026-10-23" {
		t.Errorf("got %+v, want volume 2 released 2026-10-23", got)
	}
}

// 1 ページ目が再編集版と外伝で埋まっていても、続くページの本編まで読んで最大を取ることを保証する。
// 「呪術廻戦」は 1 ページ目が「集英社リミックス」（13 巻まで）と「呪術廻戦≡」で、本編（30 巻）が入らなかった。
// 外伝（題名の直後が番号でない）とファンブックは数えない。
func TestLatest_ReadsPastRemixAndSkipsSpinOffs(t *testing.T) {
	gw := &fakeGateway{pages: [][]rakuten.Book{
		{
			b("呪術廻戦 13 人外魔境新宿決戦2総力戦", "2026年07月03日"),
			b("呪術廻戦≡ 3", "2026年05月01日"),
			b("呪術廻戦 ファントムパレード 公式ビジュアルブック", "2026年05月01日頃"),
			b("呪術廻戦 0 東京都立呪術高等専門学校", "2025年12月01日"),
		},
		{
			b("呪術廻戦 30", "2024年12月25日"),
			b("呪術廻戦 29", "2024年10月04日"),
		},
	}}
	u := volumes.NewUsecase(gw, now)

	got, err := u.Latest(context.Background(), "呪術廻戦")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.LatestVolume != 30 {
		t.Errorf("got %+v, want 30", got)
	}
	if gw.calls != 2 {
		t.Errorf("expected 2 pages to be read, got %d", gw.calls)
	}
}

// 楽天の書名の表記の揺れ（全角の英数字・括弧の巻数・句読点の直後の巻数・先頭の【特装版】）を吸収することを保証する。
func TestLatest_NormalizesTitles(t *testing.T) {
	tests := []struct {
		query string
		book  string
		want  int
	}{
		{"ダンジョン飯", "ダンジョン飯（14）", 14},
		{"ONE PIECE", "ＯＮＥ　ＰＩＥＣＥ　１１０", 110},
		{"ブルーロック", "【特装版】ブルーロック 32", 32},
		{"当て馬キャラのくせして、スパダリ王子に寵愛されています。", "当て馬キャラのくせして、スパダリ王子に寵愛されています。6", 6},
		{"天下", "天下統一 5", 0}, // 題名の続きは別の作品
	}
	for _, tt := range tests {
		t.Run(tt.book, func(t *testing.T) {
			gw := &fakeGateway{pages: [][]rakuten.Book{{b(tt.book, "2026年10月01日")}}}
			got, err := volumes.NewUsecase(gw, now).Latest(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.LatestVolume != tt.want || got.Found != (tt.want > 0) {
				t.Errorf("got %+v, want volume %d", got, tt.want)
			}
		})
	}
}

// 楽天のキーが無ければ呼ばずに Configured=false を返し、同じ作品の 2 回目はキャッシュから返すことを保証する。
// 楽天は 1.5 秒に 1 本しか送れないので、モーダルを開くたびに引き直さない。
func TestLatest_NotConfiguredAndCached(t *testing.T) {
	got, err := volumes.NewUsecase(nil, now).Latest(context.Background(), "呪術廻戦")
	if err != nil || got.Configured {
		t.Fatalf("got %+v, %v; want Configured=false", got, err)
	}

	gw := &fakeGateway{pages: [][]rakuten.Book{{b("呪術廻戦 30", "2024年12月25日")}}}
	u := volumes.NewUsecase(gw, now)
	for range 2 {
		if _, err := u.Latest(context.Background(), "呪術廻戦"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if gw.calls != 1 {
		t.Errorf("expected 1 call, got %d", gw.calls)
	}
}

// 楽天の失敗はエラーとして返すことを保証する（キャッシュには残らない）。
func TestLatest_GatewayError(t *testing.T) {
	wantErr := errors.New("429")
	_, err := volumes.NewUsecase(&fakeGateway{err: wantErr}, now).Latest(context.Background(), "呪術廻戦")
	if !errors.Is(err, wantErr) {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

// 楽天に書名が 1 冊も無いときだけ NoBooks を立てることを保証する。題名を含む本はあるが巻の番号が合わない
// （外伝だけ、など）ときは立てない。画面はこれで「単行本は見つかりませんでした」を出し分ける。
func TestLatest_NoBooks(t *testing.T) {
	got, err := volumes.NewUsecase(&fakeGateway{pages: [][]rakuten.Book{{}}}, now).Latest(context.Background(), "森野さんだるがらむ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.NoBooks || got.Found {
		t.Errorf("got %+v, want NoBooks", got)
	}

	gw := &fakeGateway{pages: [][]rakuten.Book{{b("呪術廻戦≡ 3", "2026年05月01日")}}}
	got, err = volumes.NewUsecase(gw, now).Latest(context.Background(), "呪術廻戦")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.NoBooks || got.Found {
		t.Errorf("got %+v, want neither NoBooks nor Found", got)
	}
}
