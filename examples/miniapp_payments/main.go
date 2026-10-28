// Test-only merchant lifecycle; does not run native checkout or grant goods.
package main

import (
	"context"
	"log"
	"os"

	link "github.com/linkmessengerme/console-sdk-go"
	"github.com/linkmessengerme/console-sdk-go/payments"
)

func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("set %s before running", name)
	}
	return value
}

func main() {
	client, err := link.NewClientFromFile(required("LINK_TEST_KEY_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	if client.CredentialsInfo().Environment != "test" {
		log.Fatal("this example requires Test credentials")
	}
	// Persist these operation UUIDs and the entire request in your backend before
	// the first call. Reuse them unchanged when rerunning after a lost response.
	appID := required("LINK_APP_ID")
	req := &payments.CreateOrderRequest{
		AppID: appID, OperationID: required("LINK_CREATE_OPERATION_ID"),
		MerchantOrderID: required("LINK_MERCHANT_ORDER_ID"),
		// Obtain from an authenticated, verified Mini App backend session, never an
		// arbitrary buyer identity supplied by Mini App JavaScript.
		BuyerSub:    required("LINK_VERIFIED_BUYER_SUB"),
		Product:     &payments.ProductSnapshot{SKU: "coins-100", Version: 1, Name: "100 coins", Kind: payments.ProductKindConsumable},
		Payment:     &payments.PaymentTerms{AmountMinor: 1250, Currency: required("LINK_PAYMENT_CURRENCY"), Method: required("LINK_PAYMENT_METHOD")},
		TestOutcome: payments.TestOutcomeSuccess,
	}
	cancelOperationID := required("LINK_CANCEL_OPERATION_ID")
	if cancelOperationID == req.OperationID {
		log.Fatal("use distinct persisted UUIDs for create and cancel")
	}
	ctx := context.Background()
	created, err := client.Payments.CreateOrder(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	if created.Result != payments.CreateOrderResultOK {
		log.Fatalf("business result: %s", created.Result)
	}
	orderID := created.Checkout.Order.OrderID
	current, err := client.Payments.GetOrder(ctx, &payments.GetOrderRequest{AppID: appID, OrderID: orderID})
	if err != nil {
		log.Fatal(err)
	}
	found, err := client.Payments.GetOrderByMerchantReference(ctx, &payments.GetOrderByMerchantReferenceRequest{AppID: appID, MerchantOrderID: req.MerchantOrderID})
	if err != nil {
		log.Fatal(err)
	}
	if found.Order.OrderID != orderID {
		log.Fatal("merchant reference mismatch")
	}
	cancelled, err := client.Payments.CancelOrder(ctx, &payments.CancelOrderRequest{AppID: appID, OrderID: orderID, OperationID: cancelOperationID})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("order=%s status=%s cancel_result=%s", orderID, current.Order.Status, cancelled.Result)
	// RecordFulfillment is demonstrated in README.md. Call it only after PAID and
	// an idempotent item grant + durable receipt committed in your own database.
}
