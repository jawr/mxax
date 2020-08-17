package website

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/jawr/mxax/internal/account"
	"github.com/stripe/stripe-go/v71"
	"github.com/stripe/stripe-go/v71/customer"
	"github.com/stripe/stripe-go/v71/paymentmethod"
	"github.com/stripe/stripe-go/v71/sub"
	"github.com/stripe/stripe-go/v71/webhook"
)

func (s *Site) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("ioutil.ReadAll: %v", err)
		return
	}

	event, err := webhook.ConstructEvent(b, r.Header.Get("Stripe-Signature"), os.Getenv("MXAX_STRIPE_WEBHOOK_SECRET"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("webhook.ConstructEvent: %v", err)
		return
	}

	c, err := customer.Get(event.GetObjectValue("customer"), nil)
	if err != nil {
		log.Printf("customer.Get: %v", err)
		return
	}

	log.Printf("stripe event: %s for %s", event.Type, c.ID)

	switch event.Type {

	case "invoice.paid":
		_, err = s.db.Query(
			r.Context(),
			`
			UPDATE accounts SET account_type = $1 WHERE stripe_customer_id = $2
			`,
			account.AccountTypeSubscription,
			c.ID,
		)
		if err != nil {
			log.Printf("Update account table: %s", err)
		}

	case "invoice.payment_failed":
		_, err = s.db.Query(
			r.Context(),
			`
			UPDATE accounts SET account_type = $1 WHERE stripe_customer_id = $2
			`,
			account.AccountTypeFree,
			c.ID,
		)
		if err != nil {
			log.Printf("Update account table: %s", err)
		}
	}

	return
}

func handleStripeCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PaymentMethodID string `json:"paymentMethodId"`
		CustomerID      string `json:"customerId"`
		PriceID         string `json:"priceId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("json.NewDecoder.Decode: %v", err)
		return
	}

	// Attach PaymentMethod
	params := &stripe.PaymentMethodAttachParams{
		Customer: stripe.String(req.CustomerID),
	}

	pm, err := paymentmethod.Attach(
		req.PaymentMethodID,
		params,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("paymentmethod.Attach: %v %s", err, pm.ID)
		return
	}

	// Update invoice settings default
	customerParams := &stripe.CustomerParams{
		InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(pm.ID),
		},
	}

	c, err := customer.Update(
		req.CustomerID,
		customerParams,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("customer.Update: %v %s", err, c.ID)
		return
	}

	// Create subscription
	subscriptionParams := &stripe.SubscriptionParams{
		Customer: stripe.String(req.CustomerID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price: stripe.String(req.PriceID),
			},
		},
	}

	subscriptionParams.AddExpand("latest_invoice.payment_intent")

	s, err := sub.New(subscriptionParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("sub.New: %v", err)
		return
	}

	writeJSON(w, s)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("json.NewEncoder.Encode: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, &buf); err != nil {
		log.Printf("io.Copy: %v", err)
		return
	}
}
