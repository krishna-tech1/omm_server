package infra

import "github.com/twilio/twilio-go"

func NewTwilioClient(accountSID, authToken string) *twilio.RestClient {
	if accountSID == "" || authToken == "" {
		return nil
	}

	params := twilio.ClientParams{
		Username: accountSID,
		Password: authToken,
	}
	return twilio.NewRestClientWithParams(params)
}
