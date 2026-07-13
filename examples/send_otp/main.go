package main

import (
	"context"
	"log"
	"os"
	"time"

	link "github.com/linkmessengerme/console-sdk-go"
	"github.com/linkmessengerme/console-sdk-go/otp"
)

func main() {
	// Programmatic construction. Prefer link.NewClientFromFile with the JSON key-file
	// you download from Console — it already carries client_id, private_key and the URLs.
	client, err := link.NewClient(
		"lk_client_xxxxxxxx",                                 // client_id from your key-file
		os.Getenv("LINK_PRIVATE_KEY"),                        // PEM Ed25519 private key (key-file "private_key")
		link.WithBaseURL("https://api-sdk.linkmessenger.me"), // key-file "api_base_url"
		link.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.OTP.Send(context.Background(), &otp.SendRequest{
		Phone: "15551234567",
		Code:  "123456",
	}, link.WithIdempotencyKey("signup:user-123:attempt-1"))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("request_id=%s status=%s", resp.RequestID, resp.Status)
}
