package httpadapter

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"fx-settlement-lab/go-backend/internal/adapter/inbound/http/middleware"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/webhooksigning"
	"fx-settlement-lab/go-backend/internal/port"
	"fx-settlement-lab/go-backend/internal/usecase"
)

type RouterDeps struct {
	Logger                  *zap.Logger
	GetReferenceRates       *usecase.GetReferenceRatesUsecase
	CreateQuote             *usecase.CreateQuoteUsecase
	AcceptQuote             *usecase.AcceptQuoteUsecase
	GetConversion           *usecase.GetConversionUsecase
	HandlePaymentWebhook    *usecase.HandlePaymentWebhookUsecase
	HandleTransferWebhook   *usecase.HandleTransferWebhookUsecase
	ReadyChecker            ReadyChecker
	PaymentWebhookVerifier  *webhooksigning.HMACVerifier
	TransferWebhookVerifier *webhooksigning.HMACVerifier
	CORSAllowedOrigins      []string
	Telemetry               port.Telemetry
}

func NewRouter(deps RouterDeps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(
		middleware.RequestContext(deps.Logger),
		middleware.RequestMetrics(deps.Telemetry),
		middleware.RequestLogger(),
		middleware.Recovery(),
		cors.New(cors.Config{
			AllowOrigins:     deps.CORSAllowedOrigins,
			AllowMethods:     []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-Request-ID", "Idempotency-Key", "X-Webhook-Signature"},
			ExposeHeaders:    []string{"X-Request-ID"},
			AllowCredentials: true,
		}),
		middleware.ErrorHandler(),
	)

	api := engine.Group("/api/v1")
	NewFXHandler(FXHandlerDeps{
		GetReferenceRates:       deps.GetReferenceRates,
		CreateQuote:             deps.CreateQuote,
		AcceptQuote:             deps.AcceptQuote,
		GetConversion:           deps.GetConversion,
		HandlePaymentWebhook:    deps.HandlePaymentWebhook,
		HandleTransferWebhook:   deps.HandleTransferWebhook,
		PaymentWebhookVerifier:  deps.PaymentWebhookVerifier,
		TransferWebhookVerifier: deps.TransferWebhookVerifier,
	}).RegisterRoutes(api)
	NewHealthHandler(deps.ReadyChecker).RegisterRoutes(engine)

	return engine
}
