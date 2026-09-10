package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"uuid"

	identifier2 "github.com/perfect-panel/server/internal/auth/identifier"
	token2 "github.com/perfect-panel/server/internal/auth/token"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/apple"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/facebook"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/github"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/google"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider/telegram"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthstate"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

const (
	OAuthGoogle   = "google"
	OAuthApple    = "apple"
	OAuthTelegram = "telegram"
	OAuthGithub   = "github"
	OAuthFacebook = "facebook"
	AuthEmail     = "email"
	// AuthExpire bounds how stale a Telegram widget result may be. The
	// result is a bearer credential that reaches us through a URL fragment,
	// so the window is kept to the round trip a user actually needs rather
	// than the day-long window it used to allow.
	AuthExpire = 300
	// telegramCallbackRetryGrace lets a client re-submit the same widget
	// result after a timed-out exchange; beyond it, a repeat is a replay.
	telegramCallbackRetryGrace = 60 * time.Second
)

type oauthRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}
type OAuthLoginGetTokenLogic struct {
	logger.Logger
	ctx     context.Context
	deps    OAuthLoginDependencies
	cfToken string
}

// NewOAuthLoginGetTokenLogic OAuth login get token
func NewOAuthLoginGetTokenLogic(ctx context.Context, deps OAuthLoginDependencies) *OAuthLoginGetTokenLogic {
	return &OAuthLoginGetTokenLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *OAuthLoginGetTokenLogic) OAuthLoginGetToken(req *dto.OAuthLoginGetTokenRequest, ip, userAgent string) (resp *dto.LoginResponse, err error) {
	requestID := uuid.NewV7().String()
	loginStatus := false
	var userInfo *user.User

	l.Infow("oauth login request started",
		logger.Field("request_id", requestID),
		logger.Field("method", req.Method),
		logger.Field("ip", ip),
		logger.Field("user_agent", userAgent),
	)

	defer func() {
		if auditErr := l.recordLoginStatus(loginStatus, userInfo, ip, userAgent, requestID, req.Method); auditErr != nil && err == nil {
			resp = nil
			err = auditErr
		}
	}()

	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, req.Method); err != nil {
		return nil, err
	}
	if _, ok := req.Callback.(map[string]interface{}); !ok {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "OAuth callback must be an object")
	}
	l.cfToken = req.CfToken
	userInfo, err = l.handleOAuthProvider(req, requestID, ip, userAgent)
	if err != nil {
		return nil, err
	}
	if userInfo.DeletedAt.Valid {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.UserNotExist), "user account does not exist")
	}
	if userInfo.Enable == nil || !*userInfo.Enable {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.UserDisabled), "user account is disabled")
	}

	token, err := l.generateToken(userInfo, requestID)
	if err != nil {
		return nil, err
	}

	loginStatus = true
	return &dto.LoginResponse{Token: token}, nil
}

func (l *OAuthLoginGetTokenLogic) google(req *dto.OAuthLoginGetTokenRequest, requestID, ip, userAgent string) (*user.User, error) {
	startTime := timeutil.Now()
	l.Infow("google oauth processing started",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGoogle),
	)

	var request oauthRequest
	if err := mapping.CloneMapToStruct(req.Callback.(map[string]interface{}), &request); err != nil {
		l.Errorw("failed to parse google callback data",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGoogle),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse callback data failed: %v", err)
	}

	l.Debugw("google oauth state validation started",
		logger.Field("request_id", requestID),
		logger.Field("state", request.State),
	)

	redirect, err := l.validateStateCode(OAuthGoogle, request.State, requestID)
	if err != nil {
		return nil, err
	}

	cfg, err := l.getGoogleConfig(requestID)
	if err != nil {
		return nil, err
	}

	client := google.New(&google.Config{
		ClientID:     cfg.ClientId,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirect,
	})

	l.Debugw("exchanging google authorization code for token",
		logger.Field("request_id", requestID),
		logger.Field("redirect_url", redirect),
	)

	token, err := client.Exchange(l.ctx, request.Code)
	if err != nil {
		l.Errorw("failed to exchange google authorization code",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGoogle),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "exchange token failed: %v", err)
	}

	l.Debugw("fetching google user information",
		logger.Field("request_id", requestID),
	)

	googleUserInfo, err := client.GetUserInfo(token.AccessToken)
	if err != nil {
		l.Errorw("failed to get google user info",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGoogle),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "get user info failed: %v", err)
	}

	l.Infow("google oauth processing completed",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGoogle),
		logger.Field("openid", googleUserInfo.OpenID),
		logger.Field("email", googleUserInfo.Email),
		logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
	)

	email := ""
	if googleUserInfo.VerifiedEmail {
		email = googleUserInfo.Email
	}
	return l.findOrRegisterUser(OAuthGoogle, googleUserInfo.OpenID, email, googleUserInfo.Picture, req.Invite, requestID, ip, userAgent)
}

func (l *OAuthLoginGetTokenLogic) apple(req *dto.OAuthLoginGetTokenRequest, requestID, ip, userAgent string) (*user.User, error) {
	startTime := timeutil.Now()
	l.Infow("apple oauth processing started",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthApple),
	)

	callback := req.Callback.(map[string]interface{})
	state, _ := callback["state"].(string)
	code, _ := callback["code"].(string)

	l.Debugw("apple oauth state validation started",
		logger.Field("request_id", requestID),
		logger.Field("state", state),
	)

	if _, err := l.validateStateCode(OAuthApple, state, requestID); err != nil {
		return nil, err
	}

	cfg, err := l.getAppleConfig(requestID)
	if err != nil {
		return nil, err
	}

	client, err := apple.New(apple.Config{
		ClientID:     cfg.ClientId,
		TeamID:       cfg.TeamID,
		KeyID:        cfg.KeyID,
		ClientSecret: cfg.ClientSecret,
		RedirectURI:  cfg.RedirectURL,
	})
	if err != nil {
		l.Errorw("failed to create apple client",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthApple),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "new apple client failed: %v", err)
	}

	l.Debugw("verifying apple web token",
		logger.Field("request_id", requestID),
	)

	resp, err := client.VerifyWebToken(l.ctx, code)
	if err != nil {
		l.Errorw("failed to verify apple web token",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthApple),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "verify web token failed: %v", err)
	}

	if resp.Error != "" {
		l.Errorw("apple web token verification returned error",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthApple),
			logger.Field("apple_error", resp.Error),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "verify web token failed: %v", resp.Error)
	}

	appleUnique, err := apple.GetUniqueID(resp.IDToken)
	if err != nil {
		l.Errorw("failed to get apple unique id",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthApple),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "get apple unique id failed: %v", err)
	}

	appleUserInfo, err := apple.GetClaims(resp.IDToken)
	if err != nil {
		l.Errorw("failed to get apple user claims",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthApple),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "get apple user info failed: %v", err)
	}

	email := ""
	if emailVal, ok := (*appleUserInfo)["email"]; ok && oauthClaimBool((*appleUserInfo)["email_verified"]) {
		email, _ = emailVal.(string)
	}

	l.Infow("apple oauth processing completed",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthApple),
		logger.Field("unique_id", appleUnique),
		logger.Field("email", email),
		logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
	)

	return l.findOrRegisterUser(OAuthApple, appleUnique, email, "", req.Invite, requestID, ip, userAgent)
}

func (l *OAuthLoginGetTokenLogic) telegram(req *dto.OAuthLoginGetTokenRequest, requestID, ip, userAgent string) (*user.User, error) {
	startTime := timeutil.Now()
	l.Infow("telegram oauth processing started",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthTelegram),
	)

	cfg, err := l.getTelegramConfig(requestID)
	if err != nil {
		return nil, err
	}

	encodeText, _ := req.Callback.(map[string]interface{})["tgAuthResult"].(string)
	l.Debugw("parsing telegram callback data",
		logger.Field("request_id", requestID),
		logger.Field("data_length", len(encodeText)),
	)

	callbackData, err := telegram.ParseAndValidateBase64([]byte(encodeText), cfg.BotToken)
	if err != nil {
		l.Errorw("failed to parse telegram callback data",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthTelegram),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse telegram callback failed: %v", err)
	}
	if callbackData.Id == nil || callbackData.AuthDate == nil {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "telegram callback is missing required identity fields")
	}

	now := timeutil.Now().Unix()
	l.Debugw("validating telegram auth date",
		logger.Field("request_id", requestID),
		logger.Field("auth_date", *callbackData.AuthDate),
		logger.Field("current_time", now),
	)

	const allowedClockSkew = int64(5 * 60)
	if *callbackData.AuthDate > now+allowedClockSkew || now-*callbackData.AuthDate > AuthExpire {
		l.Errorw("telegram auth date expired",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthTelegram),
			logger.Field("auth_date", *callbackData.AuthDate),
			logger.Field("current_time", now),
			logger.Field("expire_seconds", AuthExpire),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "auth date expired")
	}

	// The signature alone does not bind the result to one exchange, so the
	// payload is redeemable once (plus a short retry grace).
	if err := l.claimTelegramCallback(encodeText, requestID); err != nil {
		return nil, err
	}

	userID := fmt.Sprintf("%v", *callbackData.Id)
	avatar := ""
	if callbackData.PhotoUrl != nil {
		avatar = *callbackData.PhotoUrl
	}

	l.Infow("telegram oauth processing completed",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthTelegram),
		logger.Field("user_id", userID),
		logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
	)

	// Telegram Login does not provide an email address. Keep the account bound
	// only to the verified Telegram identity instead of inventing a fake email.
	return l.findOrRegisterUser(OAuthTelegram, userID, "", avatar, req.Invite, requestID, ip, userAgent)
}

// claimTelegramCallback enforces single use of a Telegram widget result. A
// Redis outage must not lock users out, so an unavailable store degrades to
// the signature and freshness checks alone.
func (l *OAuthLoginGetTokenLogic) claimTelegramCallback(payload, requestID string) error {
	key := fmt.Sprintf("%s:%s", config.TelegramCallbackKey, oauthstate.PayloadFingerprint(payload))
	allowed, err := oauthstate.ClaimSingleUse(l.ctx, l.deps.Redis, key,
		timeutil.Now(), telegramCallbackRetryGrace, time.Duration(AuthExpire)*time.Second)
	if err != nil {
		l.Errorw("telegram callback replay check unavailable",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthTelegram),
			logger.Field("error", err.Error()),
		)
		return nil
	}
	if !allowed {
		l.Errorw("telegram callback replayed",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthTelegram),
		)
		return errors.Wrap(xerr.NewErrCode(xerr.ERROR), "telegram callback has already been used")
	}
	return nil
}

func (l *OAuthLoginGetTokenLogic) github(req *dto.OAuthLoginGetTokenRequest, requestID, ip, userAgent string) (*user.User, error) {
	startTime := timeutil.Now()
	l.Infow("github oauth processing started",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGithub),
	)

	var request oauthRequest
	if err := mapping.CloneMapToStruct(req.Callback.(map[string]interface{}), &request); err != nil {
		l.Errorw("failed to parse github callback data",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGithub),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse callback data failed: %v", err)
	}

	l.Debugw("github oauth state validation started",
		logger.Field("request_id", requestID),
		logger.Field("state", request.State),
	)

	redirect, err := l.validateStateCode(OAuthGithub, request.State, requestID)
	if err != nil {
		return nil, err
	}

	cfg, err := l.getGithubConfig(requestID)
	if err != nil {
		return nil, err
	}

	client := github.New(&github.Config{
		ClientID:     cfg.ClientId,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirect,
	})

	l.Debugw("exchanging github authorization code for token",
		logger.Field("request_id", requestID),
		logger.Field("redirect_url", redirect),
	)

	token, err := client.Exchange(l.ctx, request.Code)
	if err != nil {
		l.Errorw("failed to exchange github authorization code",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGithub),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "exchange token failed: %v", err)
	}

	l.Debugw("fetching github user information",
		logger.Field("request_id", requestID),
	)

	githubUserInfo, err := client.GetUserInfo(token.AccessToken)
	if err != nil {
		l.Errorw("failed to get github user info",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGithub),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "get user info failed: %v", err)
	}

	l.Infow("github oauth processing completed",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGithub),
		logger.Field("openid", githubUserInfo.OpenID),
		logger.Field("email", githubUserInfo.Email),
		logger.Field("login", githubUserInfo.Login),
		logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
	)

	return l.findOrRegisterUser(OAuthGithub, fmt.Sprintf("%d", githubUserInfo.OpenID), githubUserInfo.Email, githubUserInfo.Avatar, req.Invite, requestID, ip, userAgent)
}

func (l *OAuthLoginGetTokenLogic) facebook(req *dto.OAuthLoginGetTokenRequest, requestID, ip, userAgent string) (*user.User, error) {
	startTime := timeutil.Now()
	l.Infow("facebook oauth processing started",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthFacebook),
	)

	var request oauthRequest
	if err := mapping.CloneMapToStruct(req.Callback.(map[string]interface{}), &request); err != nil {
		l.Errorw("failed to parse facebook callback data",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthFacebook),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse callback data failed: %v", err)
	}

	l.Debugw("facebook oauth state validation started",
		logger.Field("request_id", requestID),
		logger.Field("state", request.State),
	)

	redirect, err := l.validateStateCode(OAuthFacebook, request.State, requestID)
	if err != nil {
		return nil, err
	}

	cfg, err := l.getFacebookConfig(requestID)
	if err != nil {
		return nil, err
	}

	client := facebook.New(&facebook.Config{
		ClientID:     cfg.ClientId,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirect,
	})

	l.Debugw("exchanging facebook authorization code for token",
		logger.Field("request_id", requestID),
		logger.Field("redirect_url", redirect),
	)

	token, err := client.Exchange(l.ctx, request.Code)
	if err != nil {
		l.Errorw("failed to exchange facebook authorization code",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthFacebook),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "exchange token failed: %v", err)
	}

	l.Debugw("fetching facebook user information",
		logger.Field("request_id", requestID),
	)

	facebookUserInfo, err := client.GetUserInfo(token.AccessToken)
	if err != nil {
		l.Errorw("failed to get facebook user info",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthFacebook),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "get user info failed: %v", err)
	}

	l.Infow("facebook oauth processing completed",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthFacebook),
		logger.Field("openid", facebookUserInfo.OpenID),
		logger.Field("email", facebookUserInfo.Email),
		logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
	)

	// The Graph API only returns a confirmed address, so a non-empty email
	// is safe to bind as a verified email auth method.
	return l.findOrRegisterUser(OAuthFacebook, facebookUserInfo.OpenID, facebookUserInfo.Email, facebookUserInfo.Picture, req.Invite, requestID, ip, userAgent)
}

func (l *OAuthLoginGetTokenLogic) register(email, avatar, method, openid, invite, requestID, ip, userAgent string) (*user.User, error) {
	startTime := timeutil.Now()
	l.Infow("user registration started",
		logger.Field("request_id", requestID),
		logger.Field("auth_method", method),
		logger.Field("email", email),
		logger.Field("openid", openid),
	)

	if err := l.deps.Policy.EnsureRegistrationOpen(l.ctx, method); err != nil {
		return nil, err
	}
	if err := l.deps.Policy.VerifyHuman(l.ctx, l.cfToken, ip); err != nil {
		return nil, err
	}
	referer, err := l.resolveReferer(invite, requestID, method)
	if err != nil {
		return nil, err
	}
	if email != "" {
		canonicalEmail, err := identifier2.ValidateEmail(
			email,
			l.deps.Config.EmailDomainSuffixList,
			l.deps.Config.EmailEnableDomainSuffix,
		)
		if err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "OAuth email is not allowed: %v", err)
		}
		email = canonicalEmail
	}
	if err := l.deps.Policy.TakeIPPermit(l.ctx, ip); err != nil {
		return nil, err
	}

	var userInfo *user.User
	err = l.deps.Store.InIdentityTx(l.ctx, func(store repository.IdentityStore) error {
		if email != "" {
			l.Debugw("checking if email already exists",
				logger.Field("request_id", requestID),
				logger.Field("email", email),
			)
			if err := l.checkEmailExists(store, email, requestID); err != nil {
				return err
			}
		}

		l.Debugw("creating new user record",
			logger.Field("request_id", requestID),
			logger.Field("avatar", avatar),
		)

		userInfo = &user.User{Avatar: avatar, OnlyFirstPurchase: &l.deps.Config.OnlyFirstPurchase}
		if referer != nil {
			userInfo.RefererId = referer.Id
		}
		if err := store.User().Insert(l.ctx, userInfo); err != nil {
			l.Errorw("failed to create user record",
				logger.Field("request_id", requestID),
				logger.Field("error", err.Error()),
			)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create user info failed: %v", err)
		}

		userInfo.ReferCode = user.GenerateInviteCode(userInfo.Id)
		l.Debugw("updating user refer code",
			logger.Field("request_id", requestID),
			logger.Field("user_id", userInfo.Id),
			logger.Field("refer_code", userInfo.ReferCode),
		)

		if err := store.User().Update(l.ctx, userInfo); err != nil {
			l.Errorw("failed to update refer code",
				logger.Field("request_id", requestID),
				logger.Field("user_id", userInfo.Id),
				logger.Field("error", err.Error()),
			)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update refer code failed: %v", err)
		}

		if err := l.createAuthMethod(store, userInfo.Id, method, openid, requestID); err != nil {
			return err
		}

		if email != "" {
			if err := l.createAuthMethod(store, userInfo.Id, AuthEmail, email, requestID); err != nil {
				return err
			}
		}

		// Registration emits the domain event; the subscription module
		// grants the trial when it consumes it (idempotent, retried by
		// the dispatcher).
		if err := store.Outbox().Append(l.ctx, "identity.user_registered", strconv.FormatInt(userInfo.Id, 10), "{}"); err != nil {
			return err
		}
		registerLog := log.Register{
			AuthMethod: method,
			Identifier: logger.RedactedValue,
			RegisterIP: ip,
			UserAgent:  userAgent,
			Timestamp:  timeutil.Now().UnixMilli(),
		}
		content, err := registerLog.Marshal()
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "marshal registration audit: %v", err)
		}
		if err := store.Log().Insert(l.ctx, &log.SystemLog{
			Type:     log.TypeRegister.Uint8(),
			Date:     timeutil.Now().Format(time.DateOnly),
			ObjectID: userInfo.Id,
			Content:  string(content),
		}); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "record OAuth registration audit: %v", err)
		}

		return nil
	})

	if err != nil {
		l.Errorw("user registration failed",
			logger.Field("request_id", requestID),
			logger.Field("auth_method", method),
			logger.Field("error", err.Error()),
			logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
		)
		return userInfo, err
	}

	l.Infow("user registration completed successfully",
		logger.Field("request_id", requestID),
		logger.Field("user_id", userInfo.Id),
		logger.Field("auth_method", method),
		logger.Field("email", email),
		logger.Field("refer_code", userInfo.ReferCode),
		logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
	)
	return userInfo, nil
}

func (l *OAuthLoginGetTokenLogic) resolveReferer(invite, requestID, method string) (*user.User, error) {
	if invite == "" {
		if l.deps.Config.InviteForced {
			l.Errorw("registration blocked due to forced invite policy",
				logger.Field("request_id", requestID),
				logger.Field("auth_method", method),
			)
			return nil, errors.Wrap(xerr.NewErrCode(xerr.InviteCodeError), "invite code is required")
		}
		return nil, nil
	}

	referer, err := l.deps.Store.User().FindOneByReferCode(l.ctx, invite)
	if err != nil {
		l.Errorw("invalid invite code for OAuth registration",
			logger.Field("request_id", requestID),
			logger.Field("auth_method", method),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InviteCodeError), "invite code is invalid")
	}
	return referer, nil
}

func (l *OAuthLoginGetTokenLogic) checkEmailExists(store repository.IdentityStore, email, requestID string) error {
	userInfo, err := store.User().FindOneByEmail(l.ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("failed to check email existence",
			logger.Field("request_id", requestID),
			logger.Field("email", email),
			logger.Field("error", err.Error()),
		)
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "check email exists failed: %v", err)
	}
	if err == nil && userInfo != nil && userInfo.Id != 0 {
		l.Errorw("email already exists for another user",
			logger.Field("request_id", requestID),
			logger.Field("email", email),
			logger.Field("existing_user_id", userInfo.Id),
		)
		return errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "user email exist: %v", email)
	}
	l.Debugw("email availability confirmed",
		logger.Field("request_id", requestID),
		logger.Field("email", email),
	)
	return nil
}

func (l *OAuthLoginGetTokenLogic) createAuthMethod(store repository.IdentityStore, userID int64, authType, identifier, requestID string) error {
	l.Debugw("creating auth method",
		logger.Field("request_id", requestID),
		logger.Field("user_id", userID),
		logger.Field("auth_type", authType),
		logger.Field("identifier", identifier),
	)

	authMethod := &user.AuthMethods{
		UserId:         userID,
		AuthType:       authType,
		AuthIdentifier: identifier,
		Verified:       true,
	}
	if err := store.UserAuth().InsertUserAuthMethods(l.ctx, authMethod); err != nil {
		l.Errorw("failed to create auth method",
			logger.Field("request_id", requestID),
			logger.Field("user_id", userID),
			logger.Field("auth_type", authType),
			logger.Field("identifier", identifier),
			logger.Field("error", err.Error()),
		)
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create auth method failed: %v", err)
	}

	l.Debugw("auth method created successfully",
		logger.Field("request_id", requestID),
		logger.Field("user_id", userID),
		logger.Field("auth_type", authType),
		logger.Field("auth_method_id", authMethod.Id),
	)
	return nil
}

func (l *OAuthLoginGetTokenLogic) recordLoginStatus(loginStatus bool, userInfo *user.User, ip, userAgent, requestID, authType string) error {

	if userInfo != nil && userInfo.Id != 0 {
		loginLog := log.Login{
			Method:    authType,
			LoginIP:   ip,
			UserAgent: userAgent,
			Success:   loginStatus,
			Timestamp: timeutil.Now().UnixMilli(),
		}
		content, _ := loginLog.Marshal()
		if err := l.deps.Store.Log().Insert(l.ctx, &log.SystemLog{
			Type:     log.TypeLogin.Uint8(),
			Date:     timeutil.Now().Format("2006-01-02"),
			ObjectID: userInfo.Id,
			Content:  string(content),
		}); err != nil {
			l.Errorw("failed to insert login log",
				logger.Field("request_id", requestID),
				logger.Field("user_id", userInfo.Id),
				logger.Field("ip", ip),
				logger.Field("error", err.Error()),
			)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "record OAuth login audit: %v", err)
		}
	}
	return nil
}

func (l *OAuthLoginGetTokenLogic) handleOAuthProvider(req *dto.OAuthLoginGetTokenRequest, requestID, ip, userAgent string) (*user.User, error) {
	l.Debugw("handling oauth provider",
		logger.Field("request_id", requestID),
		logger.Field("provider", req.Method),
	)

	switch req.Method {
	case OAuthGoogle:
		return l.google(req, requestID, ip, userAgent)
	case OAuthApple:
		return l.apple(req, requestID, ip, userAgent)
	case OAuthTelegram:
		return l.telegram(req, requestID, ip, userAgent)
	case OAuthGithub:
		return l.github(req, requestID, ip, userAgent)
	case OAuthFacebook:
		return l.facebook(req, requestID, ip, userAgent)
	default:
		l.Errorw("unsupported oauth login method",
			logger.Field("request_id", requestID),
			logger.Field("method", req.Method),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "oauth login method not supported: %v", req.Method)
	}
}

func (l *OAuthLoginGetTokenLogic) generateToken(userInfo *user.User, requestID string) (string, error) {
	startTime := timeutil.Now()
	sessionId := uuid.NewV7().String()

	l.Debugw("generating jwt token",
		logger.Field("request_id", requestID),
		logger.Field("user_id", userInfo.Id),
		logger.Field("session_id", sessionId),
	)

	token, err := token2.NewJwtToken(
		l.deps.Config.JWTAccessSecret,
		timeutil.Now().Unix(),
		l.deps.Config.JWTAccessExpire,
		token2.WithOption("UserId", userInfo.Id),
		token2.WithOption("SessionId", sessionId),
	)
	if err != nil {
		l.Errorw("failed to generate jwt token",
			logger.Field("request_id", requestID),
			logger.Field("user_id", userInfo.Id),
			logger.Field("error", err.Error()),
		)
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "token generate error: %v", err)
	}

	sessionIdCacheKey := fmt.Sprintf("%v:%v", config.SessionIdKey, sessionId)
	if err = l.deps.Redis.Set(l.ctx, sessionIdCacheKey, userInfo.Id, time.Duration(l.deps.Config.JWTAccessExpire)*time.Second).Err(); err != nil {
		l.Errorw("failed to cache session id",
			logger.Field("request_id", requestID),
			logger.Field("user_id", userInfo.Id),
			logger.Field("session_id", sessionId),
			logger.Field("error", err.Error()),
		)
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "set session id error: %v", err)
	}

	l.Infow("jwt token generated successfully",
		logger.Field("request_id", requestID),
		logger.Field("user_id", userInfo.Id),
		logger.Field("session_id", sessionId),
		logger.Field("duration_ms", time.Since(startTime).Milliseconds()),
	)

	return token, nil
}

func (l *OAuthLoginGetTokenLogic) validateStateCode(provider, state, requestID string) (string, error) {
	if strings.TrimSpace(state) == "" {
		return "", errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "OAuth state is required")
	}
	stateKey := fmt.Sprintf("%s:%s", provider, state)
	l.Debugw("validating oauth state code",
		logger.Field("request_id", requestID),
		logger.Field("provider", provider),
	)

	redirect, err := oauthstate.Consume(l.ctx, l.deps.Redis, stateKey)
	if err != nil {
		l.Errorw("failed to validate state code",
			logger.Field("request_id", requestID),
			logger.Field("provider", provider),
			logger.Field("error", err.Error()),
		)
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "get %s state code failed: %v", provider, err)
	}

	l.Debugw("state code validated successfully",
		logger.Field("request_id", requestID),
		logger.Field("provider", provider),
		logger.Field("redirect_url", redirect),
	)
	return redirect, nil
}

func (l *OAuthLoginGetTokenLogic) getGoogleConfig(requestID string) (*auth.GoogleAuthConfig, error) {
	l.Debugw("fetching google oauth config",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGoogle),
	)

	authMethod, err := l.deps.Store.Auth().FindOneByMethod(l.ctx, OAuthGoogle)
	if err != nil {
		l.Errorw("failed to find google auth method",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGoogle),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find google auth method failed: %v", err)
	}

	var cfg auth.GoogleAuthConfig
	if err = cfg.Unmarshal(authMethod.Config); err != nil {
		l.Errorw("failed to unmarshal google config",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGoogle),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal google config failed: %v", err)
	}

	l.Debugw("google oauth config loaded successfully",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGoogle),
		logger.Field("client_id", cfg.ClientId),
	)
	return &cfg, nil
}

func (l *OAuthLoginGetTokenLogic) getAppleConfig(requestID string) (*auth.AppleAuthConfig, error) {
	l.Debugw("fetching apple oauth config",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthApple),
	)

	authMethod, err := l.deps.Store.Auth().FindOneByMethod(l.ctx, OAuthApple)
	if err != nil {
		l.Errorw("failed to find apple auth method",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthApple),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find apple auth method failed: %v", err)
	}

	var cfg auth.AppleAuthConfig
	if err = cfg.Unmarshal(authMethod.Config); err != nil {
		l.Errorw("failed to unmarshal apple config",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthApple),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal apple config failed: %v", err)
	}

	l.Debugw("apple oauth config loaded successfully",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthApple),
		logger.Field("client_id", cfg.ClientId),
		logger.Field("team_id", cfg.TeamID),
	)
	return &cfg, nil
}

func (l *OAuthLoginGetTokenLogic) getTelegramConfig(requestID string) (*auth.TelegramAuthConfig, error) {
	l.Debugw("fetching telegram oauth config",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthTelegram),
	)

	authMethod, err := l.deps.Store.Auth().FindOneByMethod(l.ctx, OAuthTelegram)
	if err != nil {
		l.Errorw("failed to find telegram auth method",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthTelegram),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find telegram auth method failed: %v", err)
	}

	var cfg auth.TelegramAuthConfig
	if err = json.Unmarshal([]byte(authMethod.Config), &cfg); err != nil {
		l.Errorw("failed to unmarshal telegram config",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthTelegram),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal telegram config failed: %v", err)
	}

	l.Debugw("telegram oauth config loaded successfully",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthTelegram),
	)
	return &cfg, nil
}

func (l *OAuthLoginGetTokenLogic) getGithubConfig(requestID string) (*auth.GithubAuthConfig, error) {
	l.Debugw("fetching github oauth config",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGithub),
	)

	authMethod, err := l.deps.Store.Auth().FindOneByMethod(l.ctx, OAuthGithub)
	if err != nil {
		l.Errorw("failed to find github auth method",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGithub),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find github auth method failed: %v", err)
	}

	var cfg auth.GithubAuthConfig
	if err = cfg.Unmarshal(authMethod.Config); err != nil {
		l.Errorw("failed to unmarshal github config",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthGithub),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal github config failed: %v", err)
	}

	l.Debugw("github oauth config loaded successfully",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthGithub),
		logger.Field("client_id", cfg.ClientId),
	)
	return &cfg, nil
}

func (l *OAuthLoginGetTokenLogic) getFacebookConfig(requestID string) (*auth.FacebookAuthConfig, error) {
	l.Debugw("fetching facebook oauth config",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthFacebook),
	)

	authMethod, err := l.deps.Store.Auth().FindOneByMethod(l.ctx, OAuthFacebook)
	if err != nil {
		l.Errorw("failed to find facebook auth method",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthFacebook),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find facebook auth method failed: %v", err)
	}

	var cfg auth.FacebookAuthConfig
	if err = cfg.Unmarshal(authMethod.Config); err != nil {
		l.Errorw("failed to unmarshal facebook config",
			logger.Field("request_id", requestID),
			logger.Field("provider", OAuthFacebook),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal facebook config failed: %v", err)
	}

	l.Debugw("facebook oauth config loaded successfully",
		logger.Field("request_id", requestID),
		logger.Field("provider", OAuthFacebook),
		logger.Field("client_id", cfg.ClientId),
	)
	return &cfg, nil
}

func oauthClaimBool(value interface{}) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func (l *OAuthLoginGetTokenLogic) findOrRegisterUser(authType, openID, email, avatar, invite, requestID, ip, userAgent string) (*user.User, error) {
	l.Debugw("finding or registering user",
		logger.Field("request_id", requestID),
		logger.Field("auth_type", authType),
		logger.Field("openid", openID),
		logger.Field("email", email),
	)

	userAuthMethod, err := l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, authType, openID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			l.Infow("user not found, starting registration",
				logger.Field("request_id", requestID),
				logger.Field("auth_type", authType),
				logger.Field("openid", openID),
				logger.Field("email", email),
			)
			return l.register(email, avatar, authType, openID, invite, requestID, ip, userAgent)
		}
		l.Errorw("failed to find user auth method by openid",
			logger.Field("request_id", requestID),
			logger.Field("auth_type", authType),
			logger.Field("openid", openID),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find user auth method by openid failed: %v", err)
	}

	l.Debugw("found existing user auth method",
		logger.Field("request_id", requestID),
		logger.Field("auth_type", authType),
		logger.Field("user_id", userAuthMethod.UserId),
	)

	userInfo, err := l.deps.Store.User().FindOne(l.ctx, userAuthMethod.UserId)
	if err != nil {
		l.Errorw("failed to find user by id",
			logger.Field("request_id", requestID),
			logger.Field("user_id", userAuthMethod.UserId),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find user info failed: %v", err)
	}

	l.Infow("existing user found successfully",
		logger.Field("request_id", requestID),
		logger.Field("user_id", userInfo.Id),
		logger.Field("auth_type", authType),
	)

	return userInfo, nil
}
