package profile

import (
	"context"
	"fmt"
	"time"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/facebook"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/github"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/google"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/telegram"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthstate"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

type BindOAuthLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Bind OAuth
func newBindOAuthLogic(ctx context.Context, deps Deps) *BindOAuthLogic {
	return &BindOAuthLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *BindOAuthLogic) BindOAuth(req *dto.BindOAuthRequest) (resp *dto.BindOAuthResponse, err error) {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, req.Method); err != nil {
		return nil, err
	}
	var uri string
	switch req.Method {
	case "google":
		uri, err = l.google(req)
	case "apple":
		uri, err = l.apple(req)
	case "telegram":
		uri, err = l.telegram(req)
	case "github":
		uri, err = l.github(req)
	case "facebook":
		uri, err = l.facebook(req)
	default:
		l.Errorw("oauth login method not support: %v", logger.Field("method", req.Method))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "oauth login method not support: %v", req.Method)
	}
	if err != nil {
		l.Errorw("error bind oauth", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "error bind oauth: %v", err.Error())
	}
	return &dto.BindOAuthResponse{
		Redirect: uri,
	}, nil
}

func (l *BindOAuthLogic) google(req *dto.BindOAuthRequest) (string, error) {
	authMethod, err := l.deps.Auth.FindOneByMethod(l.ctx, "google")
	if err != nil {
		return "", err
	}
	cfg := new(auth.GoogleAuthConfig)
	err = cfg.Unmarshal(authMethod.Config)
	if err != nil {
		l.Errorw("error unmarshal google config", logger.Field("error", err.Error()))
		return "", err
	}
	client := google.New(&google.Config{
		ClientID:     cfg.ClientId,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  req.Redirect,
	})
	// generate the state code
	code := random.KeyNew(32, 1)
	// save the state code
	err = l.deps.Redis.Set(l.ctx, fmt.Sprintf("google:%s", code), req.Redirect, 5*60*time.Second).Err()
	if err != nil {
		return "", err
	}
	uri := client.AuthCodeURL(code, oauth2.AccessTypeOffline)
	return uri, nil
}

func (l *BindOAuthLogic) facebook(req *dto.BindOAuthRequest) (string, error) {
	authMethod, err := l.deps.Auth.FindOneByMethod(l.ctx, "facebook")
	if err != nil {
		return "", err
	}
	var cfg auth.FacebookAuthConfig
	err = cfg.Unmarshal(authMethod.Config)
	if err != nil {
		l.Errorw("error unmarshal facebook config", logger.Field("error", err.Error()))
		return "", err
	}
	client := facebook.New(&facebook.Config{
		ClientID:     cfg.ClientId,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  req.Redirect,
	})
	// generate the state code
	code := random.KeyNew(32, 1)
	// save the state code
	err = l.deps.Redis.Set(l.ctx, fmt.Sprintf("facebook:%s", code), req.Redirect, 5*60*time.Second).Err()
	if err != nil {
		return "", err
	}
	return client.AuthCodeURL(code), nil
}
func (l *BindOAuthLogic) apple(req *dto.BindOAuthRequest) (string, error) {
	authMethod, err := l.deps.Auth.FindOneByMethod(l.ctx, "apple")
	if err != nil {
		return "", err
	}
	var cfg auth.AppleAuthConfig
	err = cfg.Unmarshal(authMethod.Config)
	if err != nil {
		l.Errorw("error unmarshal apple config", logger.Field("error", err.Error()))
		return "", err
	}
	// The stored redirect becomes a browser redirect in the Apple form-post
	// callback, so pin it to the configured site host.
	if err := oauthstate.ValidateRedirect(req.Redirect, l.siteHost()); err != nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "invalid redirect: %v", err)
	}
	uri := "https://appleid.apple.com/auth/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=%s&scope=name email&response_mode=form_post"
	// generate the state code
	code := random.KeyNew(32, 1)
	// save the state code
	err = l.deps.Redis.Set(l.ctx, fmt.Sprintf("apple:%s", code), req.Redirect, 5*60*time.Second).Err()
	if err != nil {
		l.Errorw("error save state code to redis", logger.Field("error", err.Error()))
		return "", err
	}
	return fmt.Sprintf(uri, cfg.ClientId, fmt.Sprintf("%s/v1/auth/oauth/callback/apple", cfg.RedirectURL), code), nil
}
func (l *BindOAuthLogic) github(req *dto.BindOAuthRequest) (string, error) {
	authMethod, err := l.deps.Auth.FindOneByMethod(l.ctx, "github")
	if err != nil {
		return "", err
	}
	var cfg auth.GithubAuthConfig
	err = cfg.Unmarshal(authMethod.Config)
	if err != nil {
		l.Errorw("error unmarshal github config", logger.Field("error", err.Error()))
		return "", err
	}
	client := github.New(&github.Config{
		ClientID:     cfg.ClientId,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  req.Redirect,
	})
	// generate the state code
	code := random.KeyNew(32, 1)
	// save the state code
	err = l.deps.Redis.Set(l.ctx, fmt.Sprintf("github:%s", code), req.Redirect, 5*60*time.Second).Err()
	if err != nil {
		return "", err
	}
	uri := client.AuthCodeURL(code, oauth2.AccessTypeOffline)
	return uri, nil
}

func (l *BindOAuthLogic) telegram(req *dto.BindOAuthRequest) (string, error) {
	authMethod, err := l.deps.Auth.FindOneByMethod(l.ctx, "telegram")
	if err != nil {
		return "", err
	}
	var cfg auth.TelegramAuthConfig
	err = cfg.Unmarshal(authMethod.Config)
	if err != nil {
		l.Errorw("error unmarshal telegram config", logger.Field("error", err.Error()))
		return "", err
	}
	// Telegram sends the signed widget result to this redirect, so pin it to
	// the configured site host rather than relying only on the allowed-URL
	// list registered with BotFather.
	if err := oauthstate.ValidateRedirect(req.Redirect, l.siteHost()); err != nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "invalid redirect: %v", err)
	}
	// Telegram Login has no OAuth state round-trip: the bind callback
	// authenticates the widget result by its HMAC signature and auth_date
	// freshness.
	uri, err := telegram.BuildTelegramOAuthURL(cfg.BotToken, req.Redirect)
	if err != nil {
		l.Errorw("error build telegram oauth url", logger.Field("error", err.Error()))
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "build telegram oauth url failed: %v", err)
	}
	return uri, nil
}

// siteHost snapshots the configured site host; the accessor is optional so
// tests may build Deps without it.
func (l *BindOAuthLogic) siteHost() string {
	if l.deps.SiteHost == nil {
		return ""
	}
	return l.deps.SiteHost()
}
