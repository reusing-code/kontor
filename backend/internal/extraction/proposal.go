package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/llm"
)

const baseCurrency = "EUR"

// Proposal inputs mirror the module create inputs (JSON names and omitempty).
type purchaseInput struct {
	ItemName      string   `json:"itemName"`
	Brand         string   `json:"brand,omitempty"`
	ArticleNumber string   `json:"articleNumber,omitempty"`
	Dealer        string   `json:"dealer,omitempty"`
	Price         *float64 `json:"price,omitempty"`
	PurchaseDate  string   `json:"purchaseDate,omitempty"`
	Comments      string   `json:"comments,omitempty"`
}

type contractInput struct {
	Name                    string   `json:"name"`
	ProductName             string   `json:"productName,omitempty"`
	Company                 string   `json:"company,omitempty"`
	ContractNumber          string   `json:"contractNumber,omitempty"`
	CustomerNumber          string   `json:"customerNumber,omitempty"`
	Price                   *float64 `json:"price,omitempty"`
	BillingInterval         string   `json:"billingInterval,omitempty"`
	StartDate               string   `json:"startDate,omitempty"`
	EndDate                 string   `json:"endDate,omitempty"`
	MinimumDurationMonths   int      `json:"minimumDurationMonths"`
	ExtensionDurationMonths int      `json:"extensionDurationMonths"`
	NoticePeriodMonths      int      `json:"noticePeriodMonths"`
	CustomerPortalURL       string   `json:"customerPortalUrl,omitempty"`
	Comments                string   `json:"comments,omitempty"`
}

type costInput struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Amount      *float64 `json:"amount,omitempty"`
	Date        string   `json:"date,omitempty"`
	Mileage     *float64 `json:"mileage,omitempty"`
	Comments    string   `json:"comments,omitempty"`
}

func noneProposal(note string) Proposal {
	return Proposal{Target: drafts.TargetNone, Input: json.RawMessage("{}"), Note: note}
}

// llmErrorNote maps errors that retrying will not fix to a note; ok is false
// for transient errors (timeouts, network, 5xx, 429, auth).
func llmErrorNote(err error) (note string, ok bool) {
	switch {
	case errors.Is(err, llm.ErrInvalidOutput):
		return NoteLLMInvalidOutput, true
	case errors.Is(err, llm.ErrRefused):
		return NoteLLMRefused, true
	case errors.Is(err, llm.ErrTruncated):
		return NoteLLMTruncated, true
	}
	if apiErr, isAPI := errors.AsType[*llm.APIError](err); isAPI {
		switch apiErr.StatusCode {
		case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
			return NoteLLMRequestRejected, true
		}
	}
	return "", false
}

// toProposal validates the model output against the offered catalog and
// converts foreign-currency amounts to EUR.
func (c *Classifier) toProposal(ctx context.Context, out Classification, cat Catalog, now time.Time) (Proposal, error) {
	if out.Kind == drafts.TargetNone || !cat.Offers(out.Kind) {
		return noneProposal(NoteUnclassified), nil
	}
	var (
		parentID string
		input    any
		money    conversion
	)
	switch out.Kind {
	case drafts.TargetPurchase:
		if out.Purchase == nil {
			return noneProposal(NoteLLMInvalidOutput), nil
		}
		p := out.Purchase
		parentID = pickOption(p.CategoryID, cat.purchaseCategories)
		in := purchaseInput{
			ItemName:      clean(p.ItemName),
			Brand:         clean(p.Brand),
			ArticleNumber: clean(p.ArticleNumber),
			Dealer:        clean(p.Dealer),
			PurchaseDate:  cleanDate(p.PurchaseDate),
		}
		money = c.convert(ctx, p.Price, p.Currency, rateDate(in.PurchaseDate, now))
		in.Price, in.Comments = money.amount, money.comment
		input = in
	case drafts.TargetContract:
		if out.Contract == nil {
			return noneProposal(NoteLLMInvalidOutput), nil
		}
		k := out.Contract
		parentID = pickOption(k.CategoryID, cat.contractCategories)
		interval := k.BillingInterval
		switch interval {
		case "monthly", "quarterly", "yearly":
		default:
			interval = ""
		}
		in := contractInput{
			Name:                    clean(k.Name),
			ProductName:             clean(k.ProductName),
			Company:                 clean(k.Company),
			ContractNumber:          clean(k.ContractNumber),
			CustomerNumber:          clean(k.CustomerNumber),
			BillingInterval:         interval,
			StartDate:               cleanDate(k.StartDate),
			EndDate:                 cleanDate(k.EndDate),
			MinimumDurationMonths:   max(k.MinimumDurationMonths, 0),
			ExtensionDurationMonths: max(k.ExtensionDurationMonths, 0),
			NoticePeriodMonths:      max(k.NoticePeriodMonths, 0),
			CustomerPortalURL:       cleanURL(k.CustomerPortalURL),
		}
		// Recurring prices are converted at the current rate.
		money = c.convert(ctx, k.Price, k.Currency, now)
		in.Price, in.Comments = money.amount, money.comment
		input = in
	case drafts.TargetCost:
		if out.Cost == nil {
			return noneProposal(NoteLLMInvalidOutput), nil
		}
		k := out.Cost
		parentID = pickOption(k.VehicleID, cat.vehicles)
		costType := k.Type
		if !validCostType(costType) || (costType == "mileage" && k.Mileage == nil) {
			costType = "misc"
		}
		in := costInput{
			Type:        costType,
			Description: clean(k.Description),
			Vendor:      clean(k.Vendor),
			Date:        cleanDate(k.Date),
			Mileage:     k.Mileage,
		}
		money = c.convert(ctx, k.Amount, k.Currency, rateDate(in.Date, now))
		in.Amount, in.Comments = money.amount, money.comment
		input = in
	}
	data, err := json.Marshal(input)
	if err != nil {
		return Proposal{}, err
	}
	p := Proposal{
		Target:    out.Kind,
		ParentID:  parentID,
		Input:     data,
		Confident: out.Confident && parentID != "" && money.currency == nil,
		Currency:  money.currency,
	}
	if money.currency != nil {
		p.Note = NoteCurrencyNotConverted
		if money.currency.Converted {
			p.Note = NoteCurrencyConverted
		}
	}
	return p, nil
}

type conversion struct {
	amount   *float64
	comment  string
	currency *Currency
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func normalizeCurrency(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "", "€", "EURO":
		return baseCurrency
	case "$", "US$":
		return "USD"
	case "£":
		return "GBP"
	}
	return s
}

// convert returns the amount in EUR. When that is not possible (rates
// disabled or unavailable, unknown code) the amount is dropped; the original
// is kept in the comment either way.
func (c *Classifier) convert(ctx context.Context, amount *float64, currency string, date time.Time) conversion {
	code := normalizeCurrency(currency)
	if amount == nil || code == baseCurrency {
		return conversion{amount: amount}
	}
	original := *amount
	if !currencyCode.MatchString(code) {
		code = "???"
	}
	notConverted := conversion{
		comment:  fmt.Sprintf("%s %.2f (not converted)", code, original),
		currency: &Currency{Code: code, Amount: original},
	}
	if code == "???" {
		return notConverted
	}
	rate, err := c.rates.Rate(ctx, code, baseCurrency, date)
	if err != nil {
		c.logger.Warn("currency conversion failed", "currency", code, "error", err)
		return notConverted
	}
	converted := math.Round(original*rate.Value*100) / 100
	return conversion{
		amount: &converted,
		comment: fmt.Sprintf("%s %.2f → EUR %.2f (ECB %s, 1 %s = %s EUR)",
			code, original, converted, rate.Date, code, strconv.FormatFloat(rate.Value, 'f', -1, 64)),
		currency: &Currency{Code: code, Amount: original, Converted: true, Rate: rate.Value, RateDate: rate.Date},
	}
}

func rateDate(date string, now time.Time) time.Time {
	if t, err := time.Parse(dateLayout, date); err == nil {
		return t
	}
	return now
}

func validCostType(t string) bool {
	switch t {
	case "service", "fuel", "insurance", "tax", "tires", "mileage", "misc":
		return true
	}
	return false
}

func pickOption(id string, opts []Option) string {
	id = strings.TrimSpace(id)
	for _, o := range opts {
		if strings.EqualFold(o.ID, id) {
			return o.ID
		}
	}
	return ""
}

func clean(s string) string {
	return strings.TrimSpace(s)
}

func cleanDate(s string) string {
	s = strings.TrimSpace(s)
	if _, err := time.Parse(dateLayout, s); err != nil {
		return ""
	}
	return s
}

func cleanURL(s string) string {
	s = strings.TrimSpace(s)
	parsed, err := url.ParseRequestURI(s)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return s
}
