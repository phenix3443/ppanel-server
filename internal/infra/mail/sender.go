package mail

import (
	"encoding/json"
	"fmt"

	"github.com/perfect-panel/server/pkg/logger"
)

type Sender interface {
	Send(to []string, subject, body string) error
}

func NewSender(platform, config, siteName string) (Sender, error) {
	switch parsePlatform(platform) {
	case SMTP:
		cfg := SMTPConfig{}
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			logger.Error("unmarshal email config failed", logger.Field("error", err.Error()), logger.Field("config", config))
			return nil, err
		}
		cfg.SiteName = siteName
		return NewSMTPClient(&cfg), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}
