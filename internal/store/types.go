package store

// TransactionType mirrors the classification vocabulary used by casparser
// (the CAMS/KFintech reference parser), so downstream code (FIFO, XIRR)
// can treat transactions from either import path identically.
type TransactionType string

const (
	Purchase         TransactionType = "PURCHASE"
	PurchaseSIP      TransactionType = "PURCHASE_SIP"
	Redemption       TransactionType = "REDEMPTION"
	SwitchIn         TransactionType = "SWITCH_IN"
	SwitchInMerger   TransactionType = "SWITCH_IN_MERGER"
	SwitchOut        TransactionType = "SWITCH_OUT"
	SwitchOutMerger  TransactionType = "SWITCH_OUT_MERGER"
	DividendPayout   TransactionType = "DIVIDEND_PAYOUT"
	DividendReinvest TransactionType = "DIVIDEND_REINVEST"
	STTTax           TransactionType = "STT_TAX"
	StampDutyTax     TransactionType = "STAMP_DUTY_TAX"
	TDSTax           TransactionType = "TDS_TAX"
	Segregation      TransactionType = "SEGREGATION"
	Reversal         TransactionType = "REVERSAL"
	Misc             TransactionType = "MISC"
	Unknown          TransactionType = "UNKNOWN"
)

// Transaction is one parsed row from a CAS statement, prior to being
// matched against the user's own Member/Account/Asset records.
type Transaction struct {
	Date        string // ISO yyyy-mm-dd
	Description string
	Amount      float64 // signed: negative for redemptions/switch-out
	Units       *float64
	Price       *float64 // NAV / unit price
	Balance     *float64 // running unit balance as printed
	Type        TransactionType
	AMC         string
	Folio       string
	Scheme      string
	ISIN        string
}
