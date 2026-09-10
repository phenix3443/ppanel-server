package routes

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	identityCommon "github.com/perfect-panel/server/internal/module/identity/transport/http/common"
	platformCommon "github.com/perfect-panel/server/internal/module/platform/transport/http/common"
	supportCommon "github.com/perfect-panel/server/internal/module/support/transport/http/common"
)

func registerCommonRoutes(router *server.Hertz, deps Dependencies) {
	commonGroupRouter := router.Group("/v1/common")
	commonGroupRouter.Use(deps.deviceMiddleware())
	{
		commonGroupRouter.GET("/ads", supportCommon.GetAdsHandler(deps.Support))
		commonGroupRouter.POST("/check_verification_code", identityCommon.CheckVerificationCodeHandler(deps.Identity))
		commonGroupRouter.GET("/client", platformCommon.GetClientHandler(deps.Platform))
		commonGroupRouter.GET("/heartbeat", platformCommon.HeartbeatHandler(deps.Platform))
		commonGroupRouter.POST("/send_code", identityCommon.SendEmailCodeHandler(deps.Identity))
		commonGroupRouter.POST("/send_sms_code", identityCommon.SendSmsCodeHandler(deps.Identity))
		commonGroupRouter.GET("/site/config", platformCommon.GetGlobalConfigHandler(deps.Platform))
		commonGroupRouter.GET("/site/privacy", platformCommon.GetPrivacyPolicyHandler(deps.Platform))
		commonGroupRouter.GET("/site/stat", platformCommon.GetStatHandler(deps.Platform))
		commonGroupRouter.GET("/site/tos", platformCommon.GetTosHandler(deps.Platform))
	}
}
