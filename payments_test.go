package link

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linkmessengerme/console-sdk-go/otp"
	"github.com/linkmessengerme/console-sdk-go/payments"
)

type paymentFixtureCase struct {
	Path     string          `json:"path"`
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
}
type paymentFixtures struct {
	Cases    map[string]paymentFixtureCase `json:"cases"`
	Business map[string][]json.RawMessage  `json:"business"`
	Errors   []struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	} `json:"errors"`
}

func loadPaymentFixtures(t *testing.T) paymentFixtures {
	t.Helper()
	data, err := os.ReadFile("testdata/payments.json")
	if err != nil {
		t.Fatal(err)
	}
	return paymentDecode[paymentFixtures](t, data)
}
func paymentDecode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func callPayment(t *testing.T, c *Client, name string, req []byte) (any, error) {
	t.Helper()
	ctx := context.Background()
	switch name {
	case "CreateOrder":
		q := paymentDecode[payments.CreateOrderRequest](t, req)
		return c.Payments.CreateOrder(ctx, &q)
	case "GetOrder":
		q := paymentDecode[payments.GetOrderRequest](t, req)
		return c.Payments.GetOrder(ctx, &q)
	case "GetOrderByMerchantReference":
		q := paymentDecode[payments.GetOrderByMerchantReferenceRequest](t, req)
		return c.Payments.GetOrderByMerchantReference(ctx, &q)
	case "CancelOrder":
		q := paymentDecode[payments.CancelOrderRequest](t, req)
		return c.Payments.CancelOrder(ctx, &q)
	case "RecordFulfillment":
		q := paymentDecode[payments.RecordFulfillmentRequest](t, req)
		return c.Payments.RecordFulfillment(ctx, &q)
	default:
		t.Fatalf("unknown method %s", name)
		return nil, nil
	}
}
func paymentHTTPResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"2"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func paymentTestClient(t *testing.T, handler userRoundTripper, opts ...Option) *Client {
	t.Helper()
	opts = append([]Option{WithBaseURL("https://api.example.test"), WithHTTPClient(&http.Client{Transport: handler}), WithNoRetries()}, opts...)
	c, err := NewClient(testClientID, testPrivPEM, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestPaymentsContract(t *testing.T) {
	fixtures := loadPaymentFixtures(t)
	for name, tc := range fixtures.Cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			c := paymentTestClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/oauth/v1/token" {
					_ = r.ParseForm()
					if r.Form.Get("scope") != "payments" {
						t.Errorf("scope=%q", r.Form.Get("scope"))
					}
					return paymentHTTPResponse(r, 200, `{"access_token":"pay","expires_in":900}`), nil
				}
				calls++
				if r.Method != "POST" || r.URL.Path != tc.Path || r.Header.Get("Authorization") != "Bearer pay" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("invalid request: %v %v", r.URL, r.Header)
				}
				body, _ := io.ReadAll(r.Body)
				if !reflect.DeepEqual(paymentDecode[any](t, body), paymentDecode[any](t, tc.Request)) {
					t.Errorf("wire request=%s, want %s", body, tc.Request)
				}
				return paymentHTTPResponse(r, 200, string(tc.Response)), nil
			})
			result, err := callPayment(t, c, name, tc.Request)
			if err != nil {
				t.Fatal(err)
			}
			var order *payments.Order
			switch out := result.(type) {
			case *payments.CreateOrderResponse:
				order = out.Checkout.Order
				if out.Checkout.MethodAvailable {
					t.Fatal("omitted bool must default false")
				}
			case *payments.GetOrderResponse:
				order = out.Order
			case *payments.GetOrderByMerchantReferenceResponse:
				order = out.Order
			case *payments.CancelOrderResponse:
				order = out.Order
			case *payments.RecordFulfillmentResponse:
				order = out.Order
				if order.PaidAt == nil {
					t.Fatal("missing paid_at")
				}
			}
			if calls != 1 || order == nil || order.Product.Version != 9223372036854775807 || order.Offer.AmountMinor != 9007199254740993 || order.Revision != 9007199254740993 || order.CreatedAt.Nanosecond() != 123456789 {
				t.Fatalf("lost contract data: %+v", order)
			}
		})
	}
}
func TestPaymentsBusinessResultsAreNotErrorsOrRetries(t *testing.T) {
	fixtures := loadPaymentFixtures(t)
	for name, results := range fixtures.Business {
		for _, body := range results {
			want := paymentDecode[map[string]any](t, body)
			t.Run(fmt.Sprint(want["result"]), func(t *testing.T) {
				calls := 0
				c := paymentTestClient(t, func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/oauth/v1/token" {
						return paymentHTTPResponse(r, 200, `{"access_token":"pay","expires_in":900}`), nil
					}
					calls++
					return paymentHTTPResponse(r, 200, string(body)), nil
				}, WithRetryConfig(RetryConfig{MaxRetries: 2, InitialBackoff: time.Nanosecond, MaxBackoff: time.Nanosecond}))
				out, err := callPayment(t, c, name, fixtures.Cases[name].Request)
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(out)
				got := paymentDecode[map[string]any](t, encoded)
				if calls != 1 || got["result"] != want["result"] {
					t.Fatalf("calls=%d response=%s", calls, encoded)
				}
				if _, exists := want["order"]; !exists && got["order"] != nil {
					t.Fatal("absent order was fabricated")
				}
			})
		}
	}
}
func TestPaymentsProtoErrors(t *testing.T) {
	fixtures := loadPaymentFixtures(t)
	for _, tc := range fixtures.Errors {
		t.Run(fmt.Sprint(tc.Status), func(t *testing.T) {
			c := paymentTestClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/oauth/v1/token" {
					return paymentHTTPResponse(r, 200, `{"access_token":"pay","expires_in":900}`), nil
				}
				return paymentHTTPResponse(r, tc.Status, string(tc.Body)), nil
			})
			_, err := callPayment(t, c, "GetOrder", fixtures.Cases["GetOrder"].Request)
			api, ok := AsAPIError(err)
			if !ok || api.StatusCode != tc.Status || api.RequestID != "req-payments" || api.Message != "payment request rejected" || api.Details["trace_id"] != "trace-payments" {
				t.Fatalf("error=%#v", err)
			}
			if (tc.Status == 401 || tc.Status == 403) != IsAuthentication(err) {
				t.Fatal("authentication mapping")
			}
			if (tc.Status == 429) != IsRateLimit(err) {
				t.Fatal("rate limit mapping")
			}
			if tc.Status == 429 && api.Details["rate_limit"].(map[string]any)["limit"] != "9007199254740993" {
				t.Fatal("lost rate limit precision")
			}
			if tc.Status == 422 && len(api.Details["field_violations"].([]map[string]any)) != 1 {
				t.Fatal("lost field violations")
			}
		})
	}
}
func TestPaymentsScopesConcurrentCacheAndRetry(t *testing.T) {
	fixtures := loadPaymentFixtures(t)
	tc := fixtures.Cases["CreateOrder"]
	var mu sync.Mutex
	tokens := map[string]int{}
	var bodies []string
	paymentCalls := 0
	retryPhase := false
	c := paymentTestClient(t, func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/oauth/v1/token" {
			_ = r.ParseForm()
			scope := r.Form.Get("scope")
			tokens[scope]++
			return paymentHTTPResponse(r, 200, fmt.Sprintf(`{"access_token":%q,"expires_in":900}`, fmt.Sprintf("%s-%d", scope, tokens[scope]))), nil
		}
		if r.URL.Path == "/sdk/v1/otp/send" {
			if r.Header.Get("Authorization") != "Bearer otp-1" {
				t.Error("OTP token polluted")
			}
			return paymentHTTPResponse(r, 200, `{"status":"sent"}`), nil
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer payments-") {
			t.Error("payment used OTP token")
		}
		if retryPhase {
			paymentCalls++
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(body))
			switch paymentCalls {
			case 1:
				return paymentHTTPResponse(r, 503, `{"code":"ERROR_CODE_UNAVAILABLE"}`), nil
			case 2:
				return paymentHTTPResponse(r, 401, `{"code":"ERROR_CODE_UNAUTHENTICATED"}`), nil
			}
		}
		return paymentHTTPResponse(r, 200, string(tc.Response)), nil
	}, WithRetryConfig(RetryConfig{MaxRetries: 2, InitialBackoff: time.Nanosecond, MaxBackoff: time.Nanosecond}))
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, err := c.OTP.Send(context.Background(), &otp.SendRequest{Phone: "12025550123", Code: "123456"})
				if err != nil {
					t.Error(err)
				}
			} else {
				_, err := callPayment(t, c, "CreateOrder", tc.Request)
				if err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	mu.Lock()
	retryPhase = true
	mu.Unlock()
	if _, err := callPayment(t, c, "CreateOrder", tc.Request); err != nil {
		t.Fatal(err)
	}
	if _, err := c.OTP.Send(context.Background(), &otp.SendRequest{Phone: "12025550123", Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tokens, map[string]int{"otp": 1, "payments": 2}) || len(bodies) != 3 || bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("tokens=%v bodies=%v", tokens, bodies)
	}
}
func TestPaymentsRejectInvalidMutationBeforeIO(t *testing.T) {
	c := paymentTestClient(t, func(r *http.Request) (*http.Response, error) {
		t.Error("unexpected I/O")
		return nil, errors.New("unexpected I/O")
	})
	req := paymentDecode[payments.CreateOrderRequest](t, loadPaymentFixtures(t).Cases["CreateOrder"].Request)
	req.OperationID = ""
	if _, err := c.Payments.CreateOrder(context.Background(), &req); !errors.Is(err, ErrInvalidPaymentRequest) {
		t.Fatal(err)
	}
	req.OperationID = "44444444-4444-4444-8444-444444444444"
	req.Payment.AmountMinor = -1
	if _, err := c.Payments.CreateOrder(context.Background(), &req); !errors.Is(err, ErrInvalidPaymentRequest) {
		t.Fatal(err)
	}
	if _, err := c.Payments.CancelOrder(context.Background(), &payments.CancelOrderRequest{}); !errors.Is(err, ErrInvalidPaymentRequest) {
		t.Fatal(err)
	}
	if _, err := c.Payments.RecordFulfillment(context.Background(), &payments.RecordFulfillmentRequest{OperationID: req.OperationID}); !errors.Is(err, ErrInvalidPaymentRequest) {
		t.Fatal(err)
	}
}
