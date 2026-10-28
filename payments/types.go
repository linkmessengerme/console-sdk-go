// Package payments contains the Mini Apps merchant API contract.
// Int64 values use decimal strings on the wire; amounts are minor currency units.
package payments

import "time"

type CreateOrderResultCode string

const (
	CreateOrderResultUnspecified              CreateOrderResultCode = "CREATE_ORDER_RESULT_CODE_UNSPECIFIED"
	CreateOrderResultOK                       CreateOrderResultCode = "CREATE_ORDER_RESULT_CODE_OK"
	CreateOrderResultPaymentAccessNotApproved CreateOrderResultCode = "CREATE_ORDER_RESULT_CODE_PAYMENT_ACCESS_NOT_APPROVED"
	CreateOrderResultPaymentAccessRevoked     CreateOrderResultCode = "CREATE_ORDER_RESULT_CODE_PAYMENT_ACCESS_REVOKED"
	CreateOrderResultMethodNotAvailable       CreateOrderResultCode = "CREATE_ORDER_RESULT_CODE_METHOD_NOT_AVAILABLE"
	CreateOrderResultIdempotencyConflict      CreateOrderResultCode = "CREATE_ORDER_RESULT_CODE_IDEMPOTENCY_CONFLICT"
)

type CancelOrderResultCode string

const (
	CancelOrderResultUnspecified         CancelOrderResultCode = "CANCEL_ORDER_RESULT_CODE_UNSPECIFIED"
	CancelOrderResultOK                  CancelOrderResultCode = "CANCEL_ORDER_RESULT_CODE_OK"
	CancelOrderResultOrderExpired        CancelOrderResultCode = "CANCEL_ORDER_RESULT_CODE_ORDER_EXPIRED"
	CancelOrderResultAlreadyPaid         CancelOrderResultCode = "CANCEL_ORDER_RESULT_CODE_ALREADY_PAID"
	CancelOrderResultIdempotencyConflict CancelOrderResultCode = "CANCEL_ORDER_RESULT_CODE_IDEMPOTENCY_CONFLICT"
	CancelOrderResultAttemptInProgress   CancelOrderResultCode = "CANCEL_ORDER_RESULT_CODE_ATTEMPT_IN_PROGRESS"
	CancelOrderResultCancelled           CancelOrderResultCode = "CANCEL_ORDER_RESULT_CODE_CANCELLED"
)

type RecordFulfillmentResultCode string

const (
	RecordFulfillmentResultUnspecified         RecordFulfillmentResultCode = "RECORD_FULFILLMENT_RESULT_CODE_UNSPECIFIED"
	RecordFulfillmentResultOK                  RecordFulfillmentResultCode = "RECORD_FULFILLMENT_RESULT_CODE_OK"
	RecordFulfillmentResultIdempotencyConflict RecordFulfillmentResultCode = "RECORD_FULFILLMENT_RESULT_CODE_IDEMPOTENCY_CONFLICT"
	RecordFulfillmentResultFulfillmentConflict RecordFulfillmentResultCode = "RECORD_FULFILLMENT_RESULT_CODE_FULFILLMENT_CONFLICT"
)

type Environment string

const (
	EnvironmentUnspecified Environment = "ENVIRONMENT_UNSPECIFIED"
	EnvironmentTest        Environment = "ENVIRONMENT_TEST"
	EnvironmentLive        Environment = "ENVIRONMENT_LIVE"
)

type ProductKind string

const (
	ProductKindUnspecified   ProductKind = "PRODUCT_KIND_UNSPECIFIED"
	ProductKindConsumable    ProductKind = "PRODUCT_KIND_CONSUMABLE"
	ProductKindNonConsumable ProductKind = "PRODUCT_KIND_NON_CONSUMABLE"
)

type TestOutcome string

const (
	TestOutcomeUnspecified  TestOutcome = "TEST_OUTCOME_UNSPECIFIED"
	TestOutcomeSuccess      TestOutcome = "TEST_OUTCOME_SUCCESS"
	TestOutcomeInsufficient TestOutcome = "TEST_OUTCOME_INSUFFICIENT"
	TestOutcomeDelayed      TestOutcome = "TEST_OUTCOME_DELAYED"
)

type OrderStatus string

const (
	OrderStatusUnspecified OrderStatus = "ORDER_STATUS_UNSPECIFIED"
	OrderStatusOpen        OrderStatus = "ORDER_STATUS_OPEN"
	OrderStatusProcessing  OrderStatus = "ORDER_STATUS_PROCESSING"
	OrderStatusPaid        OrderStatus = "ORDER_STATUS_PAID"
	OrderStatusCancelled   OrderStatus = "ORDER_STATUS_CANCELLED"
	OrderStatusExpired     OrderStatus = "ORDER_STATUS_EXPIRED"
)

type FulfillmentStatus string

const (
	FulfillmentStatusUnspecified  FulfillmentStatus = "FULFILLMENT_STATUS_UNSPECIFIED"
	FulfillmentStatusNotRequested FulfillmentStatus = "FULFILLMENT_STATUS_NOT_REQUESTED"
	FulfillmentStatusPending      FulfillmentStatus = "FULFILLMENT_STATUS_PENDING"
	FulfillmentStatusFulfilled    FulfillmentStatus = "FULFILLMENT_STATUS_FULFILLED"
)

type CreateOrderRequest struct {
	AppID string `json:"app_id"`
	// OperationID is a persisted UUID. Retry the entire request unchanged.
	OperationID     string           `json:"operation_id"`
	MerchantOrderID string           `json:"merchant_order_id"`
	BuyerSub        string           `json:"buyer_sub"`
	Product         *ProductSnapshot `json:"product,omitempty"`
	Payment         *PaymentTerms    `json:"payment,omitempty"`
	TestOutcome     TestOutcome      `json:"test_outcome,omitempty"`
}

type CreateOrderResponse struct {
	Result   CreateOrderResultCode `json:"result"`
	Checkout *Checkout             `json:"checkout,omitempty"`
}

type GetOrderRequest struct {
	AppID   string `json:"app_id"`
	OrderID string `json:"order_id"`
}

type GetOrderResponse struct {
	Order *Order `json:"order,omitempty"`
}

type GetOrderByMerchantReferenceRequest struct {
	AppID           string `json:"app_id"`
	MerchantOrderID string `json:"merchant_order_id"`
}

type GetOrderByMerchantReferenceResponse struct {
	Order *Order `json:"order,omitempty"`
}

type CancelOrderRequest struct {
	AppID   string `json:"app_id"`
	OrderID string `json:"order_id"`
	// OperationID is a persisted UUID. Retry the entire request unchanged.
	OperationID string `json:"operation_id"`
}

type CancelOrderResponse struct {
	Result CancelOrderResultCode `json:"result"`
	Order  *Order                `json:"order,omitempty"`
}

type RecordFulfillmentRequest struct {
	AppID   string `json:"app_id"`
	OrderID string `json:"order_id"`
	// OperationID is a persisted UUID. Retry the entire request unchanged.
	OperationID    string `json:"operation_id"`
	ReceiptID      string `json:"receipt_id"`
	SKU            string `json:"sku"`
	ProductVersion int64  `json:"product_version,string"`
	BuyerSub       string `json:"buyer_sub"`
}

type RecordFulfillmentResponse struct {
	Result RecordFulfillmentResultCode `json:"result"`
	Order  *Order                      `json:"order,omitempty"`
}

type ProductSnapshot struct {
	SKU     string      `json:"sku"`
	Version int64       `json:"version,string"`
	Name    string      `json:"name"`
	Kind    ProductKind `json:"kind"`
}

type PaymentTerms struct {
	// AmountMinor is a positive integer in minor currency units.
	AmountMinor int64  `json:"amount_minor,string"`
	Currency    string `json:"currency"`
	Method      string `json:"method"`
}

type Offer struct {
	OfferID string `json:"offer_id"`
	Version int64  `json:"version,string"`
	// AmountMinor is a positive integer in minor currency units.
	AmountMinor int64  `json:"amount_minor,string"`
	Currency    string `json:"currency"`
	Method      string `json:"method"`
}

type Order struct {
	OrderID           string            `json:"order_id"`
	AppID             string            `json:"app_id"`
	Environment       Environment       `json:"environment"`
	MerchantOrderID   string            `json:"merchant_order_id"`
	BuyerSub          string            `json:"buyer_sub"`
	Product           *ProductSnapshot  `json:"product,omitempty"`
	Offer             *Offer            `json:"offer,omitempty"`
	Status            OrderStatus       `json:"status"`
	FulfillmentStatus FulfillmentStatus `json:"fulfillment_status"`
	CreatedAt         *time.Time        `json:"created_at,omitempty"`
	ExpiresAt         *time.Time        `json:"expires_at,omitempty"`
	Revision          int64             `json:"revision,string"`
	PaidAt            *time.Time        `json:"paid_at,omitempty"`
}

type Checkout struct {
	Order             *Order `json:"order,omitempty"`
	CheckoutSessionID string `json:"checkout_session_id"`
	MethodAvailable   bool   `json:"method_available"`
}
