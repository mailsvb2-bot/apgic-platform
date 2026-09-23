package payments

type ProviderID string
type MethodCode string

const (
	MethodBankCard          MethodCode = "BANK_CARD"
	MethodSBP               MethodCode = "SBP"
	MethodTPay              MethodCode = "T_PAY"
	MethodSberPay           MethodCode = "SBERPAY"
	MethodTelegramStars     MethodCode = "TELEGRAM_STARS"
	MethodAppleIAP          MethodCode = "APPLE_IAP"
	MethodGooglePlayBilling MethodCode = "GOOGLE_PLAY_BILLING"
	MethodBankTransfer      MethodCode = "BANK_TRANSFER"
	MethodWallet            MethodCode = "WALLET"
)

type RailCode string

const (
	RailAPGICPaymentProvider       RailCode = "APGIC_PAYMENT_PROVIDER"
	RailStoreBilling               RailCode = "STORE_BILLING"
	RailApprovedAlternativeBilling RailCode = "APPROVED_ALTERNATIVE_BILLING"
	RailExternalPurchaseAllowed    RailCode = "EXTERNAL_PURCHASE_ALLOWED"
	RailBankTransfer               RailCode = "BANK_TRANSFER_RAIL"
	RailPurchaseDisabled           RailCode = "PURCHASE_DISABLED"
)

type Selection struct {
	ProviderID ProviderID `json:"provider_id"`
	MethodCode MethodCode `json:"method_code"`
	RailCode   RailCode   `json:"rail_code"`
}
