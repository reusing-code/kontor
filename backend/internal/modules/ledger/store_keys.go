package ledger

import (
	"fmt"

	"github.com/google/uuid"
)

// Ledger account key helpers

func ledAccKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/acc/%s", userID, id))
}

func ledAccPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/acc/", userID))
}

func idxLedAccIBANKey(userID string, iban string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/acc_iban/%s", userID, iban))
}

// Ledger category key helpers

func ledCatKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/cat/%s", userID, id))
}

func ledCatPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/cat/", userID))
}

// Ledger transaction key helpers

func ledTxnKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/txn/%s", userID, id))
}

func ledTxnPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/txn/", userID))
}

func idxLedAccTxnKey(userID string, accountID uuid.UUID, bookingDate string, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/acc_txn/%s/%s/%s", userID, accountID, bookingDate, txnID))
}

func idxLedTxnFPKey(userID string, fingerprint string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/txn_fp/%s", userID, fingerprint))
}

func idxLedImpTxnKey(userID string, batchID uuid.UUID, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/imp_txn/%s/%s", userID, batchID, txnID))
}

// Ledger import batch key helpers

func ledImpKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/imp/%s", userID, id))
}

func ledImpPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/imp/", userID))
}

func idxLedFileHashKey(userID string, sha256 string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/file_hash/%s", userID, sha256))
}

// Ledger email key helpers

func ledEmailAccKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/emailacc/%s", userID, id))
}

func ledEmailAccPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/emailacc/", userID))
}

func ledEmailOrderKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/eord/%s", userID, id))
}

func ledEmailOrderPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/eord/", userID))
}

func idxLedEmailSeenKey(userID string, accountID uuid.UUID, key string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/eseen/%s/%s", userID, accountID, key))
}

func idxLedEmailRejectKey(userID string, accountID uuid.UUID, key string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/erej/%s/%s", userID, accountID, key))
}

func idxLedEmailRejectPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/erej/%s/", userID, accountID))
}

func idxLedEmailExtractFailKey(userID string, accountID uuid.UUID, key string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/efail/%s/%s", userID, accountID, key))
}

func idxLedEmailExtractFailPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/efail/%s/", userID, accountID))
}

func idxLedEmailSeenPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/eseen/%s/", userID, accountID))
}

func idxLedEmailAccOrderKey(userID string, accountID uuid.UUID, orderID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/emailacc_eord/%s/%s", userID, accountID, orderID))
}

func idxLedEmailMsgIDKey(userID string, messageID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/eord_msgid/%s", userID, messageID))
}
