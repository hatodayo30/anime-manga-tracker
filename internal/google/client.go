// Package google は Google ログイン（OAuth 2.0 の認可コードフロー）のうち、Google と通信する部分。
// usecase/auth が定義した GoogleGateway を実装する（BACKLOG 8-b）。
//
// state の生成・照合とセッションの発行はここでは持たない。前者は Cookie を扱うハンドラ、
// 後者は usecase/auth の仕事で、ここは「認可コードを受け取って誰なのかを返す」だけにする。
package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

// userinfo は OpenID Connect の userinfo エンドポイント。sub / email / email_verified を返す。
// 古い https://www.googleapis.com/oauth2/v2/userinfo は sub ではなく id を返すので使わない。
const defaultUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

// requestTimeout はトークンの引き換えと userinfo のそれぞれの上限。利用者はコールバックの
// リダイレクトの途中で待っているので、Google が詰まったら長く待たせずに失敗させる。
const requestTimeout = 10 * time.Second

type Client struct {
	conf        *oauth2.Config
	userinfoURL string
	httpClient  *http.Client
}

var _ auth.GoogleGateway = (*Client)(nil)

// Option は NewClient の任意の設定。テストで偽の Google に向けるのに使う。
type Option func(*Client)

// WithEndpoints は認可・トークン・userinfo の URL を差し替える。
func WithEndpoints(authURL, tokenURL, userinfoURL string) Option {
	return func(c *Client) {
		c.conf.Endpoint = oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInParams}
		c.userinfoURL = userinfoURL
	}
}

// NewClient の redirectURL は Google Cloud Console に登録したものと完全一致でなければならない。
func NewClient(clientID, clientSecret, redirectURL string, opts ...Option) *Client {
	c := &Client{
		conf: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     endpoints.Google,
			// 要るのは sub と email / email_verified だけ。profile（名前・顔写真）は預からない
			Scopes: []string{"openid", "email"},
		},
		userinfoURL: defaultUserinfoURL,
		httpClient:  &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// AuthCodeURL は PKCE（RFC 7636）の S256 で code_challenge を載せる。verifier そのものは URL に出さない。
func (c *Client) AuthCodeURL(state, verifier string) string {
	return c.conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

// userinfoResponse は userinfo の応答のうち使う項目。
type userinfoResponse struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// Profile はトークンの応答に入っている id_token を読まない。アクセストークンで userinfo を叩けば、
// TLS で Google から直接受け取った値になるので、JWT の署名検証（鍵の取得・キャッシュ）が要らない。
func (c *Client) Profile(ctx context.Context, code, verifier string) (*auth.GoogleProfile, error) {
	// oauth2 は ctx に入れた http.Client でトークンの引き換えをする。入れないと http.DefaultClient
	// （タイムアウト無し）になる
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.httpClient)
	token, err := c.conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("google: exchange code: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("google: build userinfo request: %w", err)
	}
	token.SetAuthHeader(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google: userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("google: userinfo: status %d: %s", resp.StatusCode, body)
	}

	var info userinfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("google: decode userinfo: %w", err)
	}
	// sub が空のまま進むと、空文字の subject で全員が同じアカウントに入ってしまう
	if info.Sub == "" {
		return nil, errors.New("google: userinfo has no sub")
	}
	return &auth.GoogleProfile{Subject: info.Sub, Email: info.Email, EmailVerified: info.EmailVerified}, nil
}
