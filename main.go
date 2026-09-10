package main

import "github.com/perfect-panel/server/cmd"

// @title PPanel API
// @version 1.0
// @description HTTP API for PPanel's Hertz server. Application errors are returned in the response code and message fields.
// @accept json
// @produce json
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @securityDefinitions.apikey NodeSecret
// @in query
// @name secret_key
// @securityDefinitions.apikey TelegramSecret
// @in header
// @name X-Telegram-Bot-Api-Secret-Token
func main() {
	cmd.Execute()
}
