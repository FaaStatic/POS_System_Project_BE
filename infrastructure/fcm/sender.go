package fcm

import (
	"context"
	"fmt"
	"shop_project_be/internal/domain"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type Sender struct {
	client *messaging.Client
}

const channelPayment = "payment_notification"

func NewSender(ctx context.Context, credentialsPath string) (*Sender, error) {
	opt := option.WithAuthCredentialsFile(option.ServiceAccount, credentialsPath)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase app: %v", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase messaging client: %v", err)
	}
	return &Sender{client: client}, nil
}

func (s *Sender) SendToToken(ctx context.Context, token []string, p domain.Payload) ([]string, error) {
	if len(token) == 0 {
		return nil, fmt.Errorf("token is empty")
	}

	msg := &messaging.MulticastMessage{
		Tokens: token,
		Notification: &messaging.Notification{
			Title: p.Title,
			Body:  p.Body,
		},
		Data: p.Data,
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				ChannelID: channelPayment,
			},
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Category: "payment_notification",
					Sound:    "default",
				},
			},
		},
	}

	br, err := s.client.SendEachForMulticast(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("error sending message to token: %v", err)
	}

	var invalid []string
	for i, resp := range br.Responses {
		if resp.Success {
			continue
		}
		if messaging.IsUnregistered(resp.Error) || messaging.IsInvalidArgument(resp.Error) {
			invalid = append(invalid, token[i])
		}
	}

	return invalid, nil
}
