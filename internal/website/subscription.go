package website

import (
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getPostSubscription() (*route, error) {
	r := &route{
		path:    "/subscription/:code",
		methods: []string{"GET", "POST"},
	}

	type data struct {
		StripeCustomerID string
		PriceID          string
		ProductID        string
	}

	tmpl, err := s.loadTemplate("templates/pages/subscription.html")
	if err != nil {
		return r, err
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		d := data{
			PriceID:   os.Getenv("MXAX_STRIPE_PRICE_ID"),
			ProductID: os.Getenv("MXAX_STRIPE_PRODUCT_ID"),
		}

		err := s.db.QueryRow(
			req.Context(),
			`
			SELECT stripe_customer_id
			FROM accounts
			WHERE verify_code = $1 AND stripe_customer_id != ''
			`,
			ps.ByName("code"),
		).Scan(&d.StripeCustomerID)
		if err != nil {
			return err
		}

		if req.Method == "POST" {

			http.Redirect(w, req, "/thankyou/subscribe", http.StatusFound)

			return nil
		}

		return s.renderTemplate(w, tmpl, r, &d)
	}

	return r, nil
}
