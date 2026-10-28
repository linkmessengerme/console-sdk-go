package link

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/linkmessengerme/console-sdk-go/internal/transport"
	"github.com/linkmessengerme/console-sdk-go/payments"
)

// PaymentsService exposes Mini Apps merchant operations using the payments scope.
// It does not perform native checkout, OTP confirmation or the item grant itself.
type PaymentsService struct{ transport *transport.Client }

var ErrInvalidPaymentRequest = errors.New("link: invalid payment request")

const paymentOrdersPath = "/sdk/v1/miniapps/payments/orders"

var paymentUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validPaymentOperation(id string) bool {
	return paymentUUID.MatchString(id) && id != "00000000-0000-0000-0000-000000000000"
}

// CreateOrder snapshots a backend-priced purchase. Persist the request and its
// operation UUID before sending it; retry it unchanged after an uncertain result.
// A nil error does not imply success: inspect the business Result.
func (s *PaymentsService) CreateOrder(ctx context.Context, req *payments.CreateOrderRequest) (*payments.CreateOrderResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	if !validPaymentOperation(req.OperationID) || req.Product == nil || req.Payment == nil || req.Product.Version <= 0 || req.Payment.AmountMinor <= 0 {
		return nil, ErrInvalidPaymentRequest
	}
	out, err := paymentRequest[payments.CreateOrderResponse](ctx, s, "", req)
	if err == nil && (out.Result == "" || out.Result == payments.CreateOrderResultUnspecified ||
		(out.Result == payments.CreateOrderResultOK && (out.Checkout == nil || out.Checkout.Order == nil || out.Checkout.CheckoutSessionID == ""))) {
		return nil, ErrInvalidResponse
	}
	return out, err
}

func (s *PaymentsService) GetOrder(ctx context.Context, req *payments.GetOrderRequest) (*payments.GetOrderResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	out, err := paymentRequest[payments.GetOrderResponse](ctx, s, "/get", req)
	if err == nil && out.Order == nil {
		return nil, ErrInvalidResponse
	}
	return out, err
}

func (s *PaymentsService) GetOrderByMerchantReference(ctx context.Context, req *payments.GetOrderByMerchantReferenceRequest) (*payments.GetOrderByMerchantReferenceResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	out, err := paymentRequest[payments.GetOrderByMerchantReferenceResponse](ctx, s, "/by-merchant-reference", req)
	if err == nil && out.Order == nil {
		return nil, ErrInvalidResponse
	}
	return out, err
}

// CancelOrder is not a refund. Inspect Result, including AlreadyPaid and AttemptInProgress.
func (s *PaymentsService) CancelOrder(ctx context.Context, req *payments.CancelOrderRequest) (*payments.CancelOrderResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	if !validPaymentOperation(req.OperationID) {
		return nil, ErrInvalidPaymentRequest
	}
	out, err := paymentRequest[payments.CancelOrderResponse](ctx, s, "/cancel", req)
	if err == nil && (out.Result == "" || out.Result == payments.CancelOrderResultUnspecified ||
		(out.Result != payments.CancelOrderResultIdempotencyConflict && out.Order == nil)) {
		return nil, ErrInvalidResponse
	}
	return out, err
}

// RecordFulfillment records a grant already committed with a durable receipt in
// your database after observing PAID. It does not grant the item itself.
func (s *PaymentsService) RecordFulfillment(ctx context.Context, req *payments.RecordFulfillmentRequest) (*payments.RecordFulfillmentResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	if !validPaymentOperation(req.OperationID) || req.ProductVersion <= 0 {
		return nil, ErrInvalidPaymentRequest
	}
	out, err := paymentRequest[payments.RecordFulfillmentResponse](ctx, s, "/fulfillment", req)
	if err == nil && (out.Result == "" || out.Result == payments.RecordFulfillmentResultUnspecified ||
		(out.Result != payments.RecordFulfillmentResultIdempotencyConflict && out.Order == nil)) {
		return nil, ErrInvalidResponse
	}
	return out, err
}

func paymentRequest[T any](ctx context.Context, s *PaymentsService, suffix string, req any) (*T, error) {
	if s == nil || s.transport == nil {
		return nil, ErrNilClient
	}
	var out T
	if err := s.transport.DoJSON(ctx, http.MethodPost, paymentOrdersPath+suffix, req, &out, transport.RequestOptions{RetrySafe: true}); err != nil {
		return nil, err
	}
	return &out, nil
}
