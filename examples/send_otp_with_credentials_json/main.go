package main

import (
	"context"
	"log"
	"os"

	link "github.com/linkmessengerme/console-sdk-go"
	"github.com/linkmessengerme/console-sdk-go/otp"
)

func main() {
	credentialsJSON := []byte(os.Getenv("LINK_SERVICE_KEY_JSON"))
	client, err := link.NewClientFromJSON(credentialsJSON)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.OTP.Send(context.Background(), &otp.SendRequest{
		Phone: "15551234567",
		Code:  "123456",
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("request_id=%s status=%s", resp.RequestID, resp.Status)
}
