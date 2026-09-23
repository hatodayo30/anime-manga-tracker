package postgres

import "errors"

// ErrNotFound は該当する行が見つからなかったことを表す汎用エラー（メール検索・セッション検索など）。
var ErrNotFound = errors.New("not found")
