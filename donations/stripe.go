package donations

import (
	"os"
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/subscription"
)

// Stripe price IDs for recurring monthly subscriptions (production).
var stripePriceIDs = map[int]string{
	1:  "price_1QPo6pP3oIVajsTkjR41BjuL",
	2:  "price_1QK244P3oIVajsTkYcUs6kEM",
	5:  "price_1QPo7cP3oIVajsTkdGnF7kI4",
	10: "price_1QJv7GP3oIVajsTkTG7RGAUA",
	15: "price_1QK24rP3oIVajsTkwkXPms9B",
	25: "price_1QK24VP3oIVajsTk3e57kF5S",
}

func getStripeKey(test bool) string {
	if test {
		return os.Getenv("STRIPE_SECRET_KEY_TEST")
	}
	return os.Getenv("STRIPE_SECRET_KEY")
}

// CreateIntent creates a Stripe PaymentIntent for a one-time donation.
//
// @Summary Create Stripe PaymentIntent
// @Tags donations
// @Accept json
// @Produce json
// @Router /stripecreateintent [post]
func CreateIntent(c *fiber.Ctx) error {
	type CreateIntentRequest struct {
		Amount      float64 `json:"amount"`
		Test        bool    `json:"test"`
		PaymentType string  `json:"paymenttype"`
	}

	var req CreateIntentRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.PaymentType == "" {
		req.PaymentType = "card"
	}

	key := getStripeKey(req.Test)
	if key == "" {
		return c.JSON(fiber.Map{"ret": 2, "status": "Stripe not configured"})
	}
	stripe.Key = key

	// Get user ID for metadata (optional - donations can be anonymous).
	myid := user.WhoAmI(c)

	// Convert amount to pence (Stripe uses smallest currency unit).
	amountPence := int64(req.Amount * 100)

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountPence),
		Currency: stripe.String("gbp"),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}

	if myid > 0 {
		params.AddMetadata("uid", strconv.FormatUint(myid, 10))
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		return c.JSON(fiber.Map{"ret": 2, "status": "Failed to create payment intent: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"intent": pi,
	})
}

// CreateSubscription creates a Stripe subscription for a recurring monthly donation.
//
// @Summary Create Stripe subscription
// @Tags donations
// @Accept json
// @Produce json
// @Router /stripecreatesubscription [post]
func CreateSubscription(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type CreateSubRequest struct {
		Amount int  `json:"amount"`
		Test   bool `json:"test"`
	}

	var req CreateSubRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Validate amount against allowed values.
	priceID, ok := stripePriceIDs[req.Amount]
	if !ok {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid amount - must be 1, 2, 5, 10, 15, or 25"})
	}

	key := getStripeKey(req.Test)
	if key == "" {
		return c.JSON(fiber.Map{"ret": 2, "status": "Stripe not configured"})
	}
	stripe.Key = key

	// Look up user's email and name for the Stripe customer.
	db := database.DBConn

	var email string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", myid).Scan(&email)

	var fullname *string
	db.Raw("SELECT fullname FROM users WHERE id = ?", myid).Scan(&fullname)

	name := ""
	if fullname != nil {
		name = *fullname
	}

	// Create Stripe customer.
	custParams := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
	}
	custParams.AddMetadata("uid", strconv.FormatUint(myid, 10))

	cust, err := customer.New(custParams)
	if err != nil {
		return c.JSON(fiber.Map{"ret": 2, "status": "Failed to create customer: " + err.Error()})
	}

	// Create subscription.
	subParams := &stripe.SubscriptionParams{
		Customer: stripe.String(cust.ID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price: stripe.String(priceID),
			},
		},
		PaymentBehavior: stripe.String("default_incomplete"),
		PaymentSettings: &stripe.SubscriptionPaymentSettingsParams{
			SaveDefaultPaymentMethod: stripe.String("on_subscription"),
		},
	}
	subParams.AddExpand("latest_invoice.confirmation_secret")

	sub, err := subscription.New(subParams)
	if err != nil {
		return c.JSON(fiber.Map{"ret": 2, "status": "Failed to create subscription: " + err.Error()})
	}

	// Extract client secret from the latest invoice's confirmation secret.
	var clientSecret string
	if sub.LatestInvoice != nil && sub.LatestInvoice.ConfirmationSecret != nil {
		clientSecret = sub.LatestInvoice.ConfirmationSecret.ClientSecret
	}

	return c.JSON(fiber.Map{
		"ret":            0,
		"status":         "Success",
		"subscriptionId": sub.ID,
		"clientSecret":   clientSecret,
	})
}
