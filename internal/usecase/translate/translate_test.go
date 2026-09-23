package translate_test

import (
	"context"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/translate"
)

type fakeGateway struct {
	translateFunc func(ctx context.Context, text, targetLang string) (string, error)
}

func (f *fakeGateway) Translate(ctx context.Context, text, targetLang string) (string, error) {
	return f.translateFunc(ctx, text, targetLang)
}

func TestUsecase_Translate_DefaultsTargetLangToJa(t *testing.T) {
	var gotTarget string
	gw := &fakeGateway{
		translateFunc: func(ctx context.Context, text, targetLang string) (string, error) {
			gotTarget = targetLang
			return "翻訳済み", nil
		},
	}
	u := translate.NewUsecase(gw)

	got, err := u.Translate(context.Background(), "hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "翻訳済み" {
		t.Errorf("got %q, want %q", got, "翻訳済み")
	}
	if gotTarget != "ja" {
		t.Errorf("expected default target lang 'ja', got %q", gotTarget)
	}
}

func TestUsecase_Translate_PassesThroughExplicitTarget(t *testing.T) {
	var gotTarget string
	gw := &fakeGateway{
		translateFunc: func(ctx context.Context, text, targetLang string) (string, error) {
			gotTarget = targetLang
			return "", nil
		},
	}
	u := translate.NewUsecase(gw)

	if _, err := u.Translate(context.Background(), "hello", "fr"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTarget != "fr" {
		t.Errorf("expected target lang 'fr', got %q", gotTarget)
	}
}
