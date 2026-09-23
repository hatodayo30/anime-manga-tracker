// Package translate は作品モーダルのあらすじを表示言語に翻訳するユースケースを担う。
package translate

import "context"

// Gateway は Usecase が翻訳に必要とする操作を定義する。実装は internal/translate が提供する。
type Gateway interface {
	Translate(ctx context.Context, text, targetLang string) (string, error)
}

const defaultTargetLang = "ja"

type Usecase struct {
	gateway Gateway
}

func NewUsecase(gateway Gateway) *Usecase {
	return &Usecase{gateway: gateway}
}

// Translate は text を targetLang に翻訳する。targetLang が空なら日本語（ja）を既定とする。
func (u *Usecase) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if targetLang == "" {
		targetLang = defaultTargetLang
	}
	return u.gateway.Translate(ctx, text, targetLang)
}
