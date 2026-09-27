package demand

import "github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"

func newJourneyID() (string, error) {
	return persistentid.New()
}

func bookingIDForHold(holdID string) (string, error) {
	return persistentid.FromRef("booking-for-hold", holdID)
}

func checkoutInstructionIDForOrder(orderID string) (string, error) {
	return persistentid.FromRef("checkout-instruction", orderID)
}

func paymentEvidenceIDForKey(key string) (string, error) {
	return persistentid.FromRef("payment-evidence", key)
}

func refundIDForOrder(orderID string) (string, error) {
	return persistentid.FromRef("refund-for-order", orderID)
}
