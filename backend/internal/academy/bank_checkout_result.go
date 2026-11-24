package academy

// BankCheckoutRedirect is a transport response from the existing canonical
// Commerce Bank checkout. It NEVER proves capture, enrollment or entitlement.
type BankCheckoutRedirect struct {
	OrderID    string `json:"order_id"`
	PaymentURL string `json:"payment_url"`
	State      string `json:"state"`
}
