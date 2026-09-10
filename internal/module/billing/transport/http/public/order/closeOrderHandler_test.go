package order

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/perfect-panel/server/internal/module/billing"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
)

type unconfirmedCloseService struct{ billing.Service }

func (unconfirmedCloseService) CloseOrder(context.Context, *dto.CloseOrderRequest) error {
	return fmt.Errorf("gateway still confirming: %w", billing.ErrGatewayUnconfirmed)
}

func TestCloseOrderReportsUnconfirmedPaymentConflict(t *testing.T) {
	engine := server.Default()
	ctx := engine.NewContext()
	ctx.Request.Header.SetMethod(http.MethodPost)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.SetBodyString(`{"orderNo":"order-1"}`)
	CloseOrderHandler(unconfirmedCloseService{})(context.Background(), ctx)
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
		t.Fatal(err)
	}
	if ctx.Response.StatusCode() != http.StatusOK || response.Code != 409 || response.Message != "PAYMENT_STATUS_UNCONFIRMED" {
		t.Fatalf("unexpected conflict response: %s", ctx.Response.Body())
	}
}
