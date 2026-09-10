package sms

import (
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
)

type Dependencies struct {
	Store  repository.Store
	Mobile func() config.MobileConfig
	Model  func() string
}
