package link

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkmessengerme/console-sdk-go/otp"
)

const userInfoBase = "https://regional-api.example.test"

var miniToken = "ma_" + strings.Repeat("x", 43)

type userRoundTripper func(*http.Request) (*http.Response, error)

func (fn userRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func userResponse(r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: r,
	}
}

func userTestClient(t *testing.T, fn userRoundTripper, opts ...Option) *Client {
	t.Helper()
	options := []Option{WithBaseURL(userInfoBase), WithHTTPClient(&http.Client{Transport: fn})}
	c, err := NewClient(testClientID, testPrivPEM, append(options, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUserInfoProductsAndFreshFields(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(userInfoBase)
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "unrelated"}})
	var calls int
	httpClient := &http.Client{Jar: jar, Transport: userRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Host != u.Host || r.URL.RawQuery != "" || r.Body != nil {
			t.Errorf("unexpected UserInfo request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("Accept") != "application/json" {
			t.Error("UserInfo must omit cookies and request uncached JSON")
		}
		switch calls {
		case 1:
			if r.URL.Path != "/oauth/v1/userinfo" || r.Header.Get("Authorization") != "Bearer user-token" {
				t.Error("Login must use the supplied user bearer")
			}
			return userResponse(r, `{"sub":"u1","name":"Alex","preferred_username":"alex","picture":"https://img.example.test/a","phone_number":"+12025550123","phone_number_verified":false}`), nil
		case 2:
			if r.URL.Path != "/miniapps/v1/userinfo" || r.Header.Get("Authorization") != "Bearer "+miniToken {
				t.Error("Mini Apps must use the supplied ma_ bearer")
			}
			return userResponse(r, `{"sub":"u1","profile":{"name":"Alex","username":"alex","avatar_url":"https://img.example.test/a"},"phone_number":"+12025550123"}`), nil
		default:
			return userResponse(r, `{"sub":"u1","phone_number_verified":true}`), nil
		}
	})}
	c := userTestClient(t, nil, WithHTTPClient(httpClient))
	login, err := c.Login.GetUserInfo(context.Background(), "user-token", WithExpectedSubject("u1"))
	if err != nil || login.Name == nil || *login.Name != "Alex" || login.PreferredUsername == nil || login.Picture == nil ||
		login.PhoneNumber == nil || *login.PhoneNumber != "+12025550123" || login.PhoneNumberVerified == nil || *login.PhoneNumberVerified {
		t.Fatalf("Login mapping failed: %+v, %v", login, err)
	}
	mini, err := c.MiniApps.GetUserInfo(context.Background(), miniToken, WithExpectedSubject("u1"))
	if err != nil || mini.Profile == nil || mini.Profile.AvatarURL == nil || mini.PhoneNumber == nil || mini.PhoneNumberVerified != nil {
		t.Fatalf("Mini App mapping failed: %+v, %v", mini, err)
	}
	fresh, err := c.Login.GetUserInfo(context.Background(), "user-token")
	if err != nil || fresh.PhoneNumber != nil || fresh.PhoneNumberVerified != nil || fresh.Name != nil || calls != 3 {
		t.Fatalf("old data leaked into fresh response: %+v, %v, %d calls", fresh, err, calls)
	}
	if httpClient.Jar != jar {
		t.Fatal("UserInfo mutated the supplied HTTP client")
	}
}

func TestUserInfoConcurrentUsersDoNotChangeOTPToken(t *testing.T) {
	var tokenCalls atomic.Int32
	c := userTestClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/oauth/v1/token":
			tokenCalls.Add(1)
			return userResponse(r, `{"access_token":"app-token","token_type":"Bearer","expires_in":900}`), nil
		case "/sdk/v1/otp/send":
			if r.Header.Get("Authorization") != "Bearer app-token" {
				t.Error("user token contaminated OTP")
			}
			return userResponse(r, `{"request_id":"r1","phone":"+12025550123","status":"queued"}`), nil
		default:
			return userResponse(r, fmt.Sprintf(`{"sub":%q}`, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))), nil
		}
	})
	send := func() {
		if _, err := c.OTP.Send(context.Background(), &otp.SendRequest{Phone: "+12025550123", Code: "123456"}); err != nil {
			t.Fatal(err)
		}
	}
	send()
	var wg sync.WaitGroup
	for _, sub := range []string{"alice", "bob"} {
		wg.Add(1)
		go func(sub string) {
			defer wg.Done()
			got, err := c.Login.GetUserInfo(context.Background(), sub, WithExpectedSubject(sub))
			if err != nil || got.Sub != sub {
				t.Errorf("user isolation failed: %v", err)
			}
		}(sub)
	}
	wg.Wait()
	send()
	if tokenCalls.Load() != 1 {
		t.Fatal("UserInfo must not request or invalidate app tokens")
	}
}

func TestUserInfoHTTPFailureIsSanitizedAndNeverRetried(t *testing.T) {
	for _, status := range []int{401, 403, 429, 503, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			c := userTestClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				resp := userResponse(r, `secret-token +12025550123`)
				resp.StatusCode = status
				resp.Header.Set("Location", "https://elsewhere.example.test")
				return resp, nil
			})
			_, err := c.Login.GetUserInfo(context.Background(), "secret-token")
			api, ok := AsAPIError(err)
			if !ok || api.StatusCode != status || calls != 1 {
				t.Fatalf("unexpected error/retry: %v, %d calls", err, calls)
			}
			if (status == 401 || status == 403) && !IsAuthentication(err) || status == 429 && !IsRateLimit(err) {
				t.Fatalf("wrong error class: %T", err)
			}
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "+12025550123") {
				t.Fatal("error contains sensitive data")
			}
		})
	}
}

func TestUserInfoRejectsBadResponse(t *testing.T) {
	for _, body := range []string{
		`{}`, `null`, `[]`, `{"sub":123}`, `{"sub":"other"}`, `{"sub":"u1","name":{}}`,
		`{"sub":"u1","phone_number":123}`, `{"sub":"u1","phone_number":"12025550123"}`,
		`{"sub":"u1","phone_number_verified":"true"}`, "not-json", "\xff",
		`{"sub":"u1","extra":"` + strings.Repeat("x", 64*1024) + `"}`,
	} {
		c := userTestClient(t, func(r *http.Request) (*http.Response, error) { return userResponse(r, body), nil })
		if _, err := c.Login.GetUserInfo(context.Background(), "token", WithExpectedSubject("u1")); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid response accepted (length %d): %v", len(body), err)
		}
	}
	for _, body := range []string{`{"sub":"u1","profile":[]}`, `{"sub":"u1","profile":{"avatar_url":123}}`} {
		c := userTestClient(t, func(r *http.Request) (*http.Response, error) { return userResponse(r, body), nil })
		if _, err := c.MiniApps.GetUserInfo(context.Background(), miniToken); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid profile accepted: %v", err)
		}
	}
	for _, kind := range []string{"content-type", "content-length", "response-url"} {
		t.Run(kind, func(t *testing.T) {
			c := userTestClient(t, func(r *http.Request) (*http.Response, error) {
				res := userResponse(r, `{"sub":"u1"}`)
				switch kind {
				case "content-type":
					res.Header.Set("Content-Type", "text/html")
				case "content-length":
					res.ContentLength = 65537
				case "response-url":
					res.Request, _ = http.NewRequest("GET", "https://elsewhere.example.test", nil)
				}
				return res, nil
			})
			if _, err := c.Login.GetUserInfo(context.Background(), "token"); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("invalid response accepted: %v", err)
			}
		})
	}
}

func TestUserInfoInputValidationBeforeIO(t *testing.T) {
	noIO := userRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected HTTP request")
		return nil, errors.New("unexpected I/O")
	})
	c := userTestClient(t, noIO)
	for _, token := range []string{"", "Bearer token", "token\r\nHeader: value", strings.Repeat("x", 8193)} {
		if _, err := c.Login.GetUserInfo(context.Background(), token); !errors.Is(err, ErrInvalidAccessToken) {
			t.Fatalf("invalid token accepted: %v", err)
		}
	}
	if _, err := c.MiniApps.GetUserInfo(context.Background(), "oauth-token"); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatal("invalid Mini App token accepted")
	}
	if _, err := c.Login.GetUserInfo(context.Background(), "token", WithExpectedSubject("")); !errors.Is(err, ErrInvalidSubject) {
		t.Fatal("empty expected subject accepted")
	}
	for _, base := range []string{"http://api.example.test", "https://user:password@api.example.test", userInfoBase + "/prefix", userInfoBase + "?key=secret"} {
		c := userTestClient(t, noIO, WithBaseURL(base))
		if _, err := c.Login.GetUserInfo(context.Background(), "token"); !errors.Is(err, ErrInvalidUserInfoURL) {
			t.Errorf("unsafe origin accepted: %s (%v)", base, err)
		}
	}
}

func TestUserInfoNetworkFailureAndCancellation(t *testing.T) {
	c := userTestClient(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("secret-token +12025550123") })
	_, err := c.Login.GetUserInfo(context.Background(), "secret-token")
	var connection *ConnectionError
	if !errors.As(err, &connection) || strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "+12025550123") {
		t.Fatalf("network error not sanitized: %v", err)
	}
	c = userTestClient(t, func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := c.Login.GetUserInfo(ctx, "token"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline not preserved: %v", err)
	}
}
