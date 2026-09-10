package telegram

import (
	"context"
	"html/template"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/app/server/render"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func TestOAuth_rendersTelegramHTML_whenRouteIsRequested(t *testing.T) {
	// Given
	router := server.Default()
	templates := template.Must(template.New("telegram.html").Parse("{{.title}}: {{.message}}"))
	router.SetHTMLTemplate(templates)
	router.GET("/telegram", func(_ context.Context, ctx *app.RequestContext) {
		ctx.HTML(http.StatusOK, "telegram.html", utils.H{
			"title":   "Hertz HTML Example",
			"message": "Hello, Hertz!",
		})
	})
	router.GET("/auth/telegram/callback", func(_ context.Context, ctx *app.RequestContext) {
		ctx.String(http.StatusOK, "callback")
	})
	if err := router.Init(); err != nil {
		t.Fatalf("initialize native Hertz router: %v", err)
	}
	request := router.NewContext()
	request.HTMLRender = render.HTMLProduction{Template: templates}
	request.Request.SetRequestURI("/telegram")
	request.Request.Header.SetMethod(http.MethodGet)

	// When
	router.ServeHTTP(context.Background(), request)

	// Then
	if status := request.Response.StatusCode(); status != http.StatusOK {
		t.Fatalf("expected HTML status %d, got %d", http.StatusOK, status)
	}
}

func TestBase64(t *testing.T) {
	text := "eyJpZCI6ODI0NjI2ODAzLCJmaXJzdF9uYW1lIjoiQ2hhbmcgbHVlIiwibGFzdF9uYW1lIjoiVHNlbiIsInVzZXJuYW1lIjoidGVuc2lvbl9jIiwicGhvdG9fdXJsIjoiaHR0cHM6XC9cL3QubWVcL2lcL3VzZXJwaWNcLzMyMFwvYU1LNkhEc0pqc2V1YldRYmt2NGlYOHZCRUF6N0hWU3g3dkFuRDBLZ0tFVS5qcGciLCJhdXRoX2RhdGUiOjE3Mzc4MTkwNzQsImhhc2giOiI5M2I1ZDg3Zjc3NjE2YjBjMTM0OTAxYmYwMDg3MTc4YjJiYmZlYzA1MTlkMWVmMDJhZjFjMGNlOTAzM2ZiNGFlIn0"
	var token = "7651491571:AAEVQma6niHhtqEYDowAEpPo6Fq69BWvRU8"

	data, err := ParseAndValidateBase64([]byte(text), token)
	if err != nil {
		t.Error(err)
	}
	t.Log(*data.Id)

}
