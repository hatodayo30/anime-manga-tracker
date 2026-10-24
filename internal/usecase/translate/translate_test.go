package translate_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/translate"
)

type fakeGateway struct {
	result string
	err    error

	calls     int
	gotText   string
	gotTarget string
}

func (f *fakeGateway) Translate(ctx context.Context, text, targetLang string) (string, error) {
	f.calls++
	f.gotText = text
	f.gotTarget = targetLang
	return f.result, f.err
}

type savedKey struct {
	hash string
	lang string
}

type fakeRepo struct {
	saved   map[savedKey]string
	findErr error
	saveErr error

	saveCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{saved: map[savedKey]string{}}
}

func (f *fakeRepo) Find(ctx context.Context, sourceSHA256 []byte, targetLang string) (string, error) {
	if f.findErr != nil {
		return "", f.findErr
	}
	t, ok := f.saved[savedKey{string(sourceSHA256), targetLang}]
	if !ok {
		return "", translate.ErrNotFound
	}
	return t, nil
}

func (f *fakeRepo) Save(ctx context.Context, sourceSHA256 []byte, targetLang, translated string) error {
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved[savedKey{string(sourceSHA256), targetLang}] = translated
	return nil
}

func keyOf(text, lang string) savedKey {
	sum := sha256.Sum256([]byte(text))
	return savedKey{string(sum[:]), lang}
}

// 翻訳先が空なら日本語を既定にすることを保証する。
func TestUsecase_Translate_DefaultsTargetLangToJa(t *testing.T) {
	fallback := &fakeGateway{result: "翻訳済み"}
	u := translate.NewUsecase(newFakeRepo(), nil, fallback)

	got, err := u.Translate(context.Background(), "hello", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "翻訳済み" {
		t.Errorf("got %q, want %q", got, "翻訳済み")
	}
	if fallback.gotTarget != "ja" {
		t.Errorf("expected default target lang 'ja', got %q", fallback.gotTarget)
	}
}

// 明示した翻訳先がそのまま渡ることを保証する。
func TestUsecase_Translate_PassesThroughExplicitTarget(t *testing.T) {
	fallback := &fakeGateway{}
	u := translate.NewUsecase(newFakeRepo(), nil, fallback)

	if _, err := u.Translate(context.Background(), "hello", "fr", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fallback.gotTarget != "fr" {
		t.Errorf("expected target lang 'fr', got %q", fallback.gotTarget)
	}
}

// 空白だけの原文では上流を 1 本も呼ばないことを保証する（AI に空の翻訳を課金させない）。
func TestUsecase_Translate_BlankTextCallsNothing(t *testing.T) {
	ai, fallback := &fakeGateway{}, &fakeGateway{}
	u := translate.NewUsecase(newFakeRepo(), ai, fallback)

	got, err := u.Translate(context.Background(), "  \n ", "ja", true)
	if err != nil || got != "" {
		t.Fatalf("got (%q, %v), want empty", got, err)
	}
	if ai.calls != 0 || fallback.calls != 0 {
		t.Errorf("expected no upstream calls, got ai=%d fallback=%d", ai.calls, fallback.calls)
	}
}

// 保存済みの AI 翻訳は、未ログインでも上流を呼ばずにそのまま返すことを保証する。
func TestUsecase_Translate_ReturnsSavedTranslationEvenWithoutLogin(t *testing.T) {
	repo := newFakeRepo()
	repo.saved[keyOf("hello", "ja")] = "保存済み"
	ai, fallback := &fakeGateway{}, &fakeGateway{}
	u := translate.NewUsecase(repo, ai, fallback)

	got, err := u.Translate(context.Background(), "hello", "ja", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "保存済み" {
		t.Errorf("got %q, want %q", got, "保存済み")
	}
	if ai.calls != 0 || fallback.calls != 0 {
		t.Errorf("expected no upstream calls, got ai=%d fallback=%d", ai.calls, fallback.calls)
	}
}

// 未ログインなら AI を呼ばず MyMemory を使い、その結果は保存しないことを保証する
// （連打で請求が積み上がらないこと、低品質な訳で AI 翻訳の機会を潰さないこと）。
func TestUsecase_Translate_WithoutLoginUsesFallbackOnly(t *testing.T) {
	repo := newFakeRepo()
	ai, fallback := &fakeGateway{result: "AI訳"}, &fakeGateway{result: "機械訳"}
	u := translate.NewUsecase(repo, ai, fallback)

	got, err := u.Translate(context.Background(), "hello", "ja", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "機械訳" {
		t.Errorf("got %q, want %q", got, "機械訳")
	}
	if ai.calls != 0 {
		t.Errorf("expected AI not to be called, got %d calls", ai.calls)
	}
	if repo.saveCalls != 0 {
		t.Errorf("expected fallback result not to be saved, got %d saves", repo.saveCalls)
	}
}

// ログイン済みで未保存なら AI で訳し、原文の SHA-256 と翻訳先言語をキーに保存することを保証する。
func TestUsecase_Translate_WithLoginUsesAIAndSaves(t *testing.T) {
	repo := newFakeRepo()
	ai, fallback := &fakeGateway{result: "AI訳"}, &fakeGateway{}
	u := translate.NewUsecase(repo, ai, fallback)

	got, err := u.Translate(context.Background(), "hello", "ja", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "AI訳" {
		t.Errorf("got %q, want %q", got, "AI訳")
	}
	if fallback.calls != 0 {
		t.Errorf("expected fallback not to be called, got %d calls", fallback.calls)
	}
	if saved := repo.saved[keyOf("hello", "ja")]; saved != "AI訳" {
		t.Errorf("expected AI result saved under sha256(text)+lang, got %q", saved)
	}

	// 2 回目は保存済みを返し、AI を呼び直さない
	if _, err := u.Translate(context.Background(), "hello", "ja", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ai.calls != 1 {
		t.Errorf("expected AI to be called once, got %d", ai.calls)
	}
}

// AI が失敗したら MyMemory に落とし、何も保存しないことを保証する（失敗の扱いの方針）。
func TestUsecase_Translate_AIFailureFallsBack(t *testing.T) {
	repo := newFakeRepo()
	ai := &fakeGateway{err: errors.New("budget exceeded")}
	fallback := &fakeGateway{result: "機械訳"}
	u := translate.NewUsecase(repo, ai, fallback)

	got, err := u.Translate(context.Background(), "hello", "ja", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "機械訳" {
		t.Errorf("got %q, want %q", got, "機械訳")
	}
	if repo.saveCalls != 0 {
		t.Errorf("expected nothing saved, got %d saves", repo.saveCalls)
	}
}

// API キー未設定（AI が nil）の環境では、ログイン済みでも MyMemory で動くことを保証する。
func TestUsecase_Translate_NilAIUsesFallback(t *testing.T) {
	fallback := &fakeGateway{result: "機械訳"}
	u := translate.NewUsecase(newFakeRepo(), nil, fallback)

	got, err := u.Translate(context.Background(), "hello", "ja", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "機械訳" {
		t.Errorf("got %q, want %q", got, "機械訳")
	}
}

// 保存先の読み取りに失敗しても翻訳は止めず、未保存として AI に進むことを保証する。
func TestUsecase_Translate_FindErrorDoesNotStopTranslation(t *testing.T) {
	repo := newFakeRepo()
	repo.findErr = errors.New("db down")
	ai := &fakeGateway{result: "AI訳"}
	u := translate.NewUsecase(repo, ai, &fakeGateway{})

	got, err := u.Translate(context.Background(), "hello", "ja", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "AI訳" {
		t.Errorf("got %q, want %q", got, "AI訳")
	}
}

// 保存に失敗しても、手元にある AI 翻訳は返すことを保証する。
func TestUsecase_Translate_SaveErrorStillReturnsTranslation(t *testing.T) {
	repo := newFakeRepo()
	repo.saveErr = errors.New("db down")
	ai := &fakeGateway{result: "AI訳"}
	u := translate.NewUsecase(repo, ai, &fakeGateway{})

	got, err := u.Translate(context.Background(), "hello", "ja", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "AI訳" {
		t.Errorf("got %q, want %q", got, "AI訳")
	}
}
