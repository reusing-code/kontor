package enablebanking

import "time"

// ASPSP is a bank offered by Enable Banking.
type ASPSP struct {
	Name    string `json:"name"`
	Country string `json:"country"`
	Logo    string `json:"logo"`
	// MaximumConsentValidity is the longest consent in seconds; 0 when unknown.
	MaximumConsentValidity int64 `json:"maximum_consent_validity"`
}

type AuthRequest struct {
	ASPSPName       string
	ASPSPCountry    string
	State           string
	RedirectURL     string
	ValidUntil      time.Time
	ExtendedHistory bool
}

type SessionAccount struct {
	UID                string
	IBAN               string
	Currency           string
	Name               string
	IdentificationHash string
}

type Session struct {
	ID         string
	ASPSP      ASPSP
	ValidUntil time.Time
	Accounts   []SessionAccount
}

type SessionStatus struct {
	Status string `json:"status"`
}

type TransactionsQuery struct {
	AccountUID      string
	DateFrom        string
	DateTo          string
	Strategy        string
	ContinuationKey string
}

type Party struct {
	Name string `json:"name"`
}

type AccountRef struct {
	IBAN string `json:"iban"`
}

type Amount struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

type Transaction struct {
	EntryReference        string      `json:"entry_reference,omitempty"`
	TransactionAmount     Amount      `json:"transaction_amount"`
	Creditor              *Party      `json:"creditor,omitempty"`
	CreditorAccount       *AccountRef `json:"creditor_account,omitempty"`
	Debtor                *Party      `json:"debtor,omitempty"`
	DebtorAccount         *AccountRef `json:"debtor_account,omitempty"`
	CreditDebitIndicator  string      `json:"credit_debit_indicator"`
	Status                string      `json:"status"`
	BookingDate           string      `json:"booking_date,omitempty"`
	ValueDate             string      `json:"value_date,omitempty"`
	TransactionDate       string      `json:"transaction_date,omitempty"`
	RemittanceInformation []string    `json:"remittance_information,omitempty"`
}

// Balance is one balance of an account; BalanceType is the ISO 20022 code
// (CLBD closing booked, ITBD interim booked, XPCD expected, CLAV/ITAV
// available, ...). The dates are kept as text so an odd value cannot fail the
// whole response.
type Balance struct {
	Name                     string `json:"name"`
	BalanceAmount            Amount `json:"balance_amount"`
	BalanceType              string `json:"balance_type"`
	LastChangeDateTime       string `json:"last_change_date_time,omitempty"`
	ReferenceDate            string `json:"reference_date,omitempty"`
	LastCommittedTransaction string `json:"last_committed_transaction,omitempty"`
}

type TransactionsPage struct {
	Transactions    []Transaction `json:"transactions"`
	ContinuationKey string        `json:"continuation_key"`
}
