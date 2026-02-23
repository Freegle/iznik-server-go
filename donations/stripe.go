package donations

import (
	"log"
	"os"
	"strconv"
	"sync"

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

// stripeMu protects stripe.Key from concurrent access. The stripe-go package
// uses a global key variable which is not safe for concurrent use with
// different keys (test vs live).
var stripeMu sync.Mutex

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
// @Security BearerAuth
// @Router /stripecreateintent [post]
func CreateIntent(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

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

	// Validate amount: minimum 30p (Stripe minimum for GBP), maximum £250.
	if req.Amount < 0.30 || req.Amount > 250.00 {
		return fiber.NewError(fiber.StatusBadRequest, "Amount must be between 0.30 and 250.00")
	}

	key := getStripeKey(req.Test)
	if key == "" {
		return fiber.NewError(fiber.StatusInternalServerError, "Payment processing not available")
	}

	// Convert amount to pence (Stripe uses smallest currency unit).
	amountPence := int64(req.Amount * 100)

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountPence),
		Currency: stripe.String("gbp"),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}

	params.AddMetadata("uid", strconv.FormatUint(myid, 10))

	log.Printf("Creating PaymentIntent for user %d, amount %d pence", myid, amountPence)

	// Protect stripe.Key from concurrent access.
	stripeMu.Lock()
	stripe.Key = key
	pi, err := paymentintent.New(params)
	stripeMu.Unlock()

	if err != nil {
		log.Printf("Stripe PaymentIntent creation failed for user %d: %v", myid, err)
		return fiber.NewError(fiber.StatusInternalServerError, "Payment processing failed")
	}

	log.Printf("PaymentIntent created: %s for user %d", pi.ID, myid)

	return c.JSON(fiber.Map{
		"id":           pi.ID,
		"clientSecret": pi.ClientSecret,
	})
}

// CreateSubscription creates a Stripe subscription for a recurring monthly donation.
//
// @Summary Create Stripe subscription
// @Tags donations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /stripecreatesubscription [post]
func CreateSubscription(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
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
		return fiber.NewError(fiber.StatusBadRequest, "Invalid amount - must be 1, 2, 5, 10, 15, or 25")
	}

	key := getStripeKey(req.Test)
	if key == "" {
		return fiber.NewError(fiber.StatusInternalServerError, "Payment processing not available")
	}

	// Look up user's email and name in parallel.
	db := database.DBConn

	var email string
	var fullname *string
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", myid).Scan(&email)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT fullname FROM users WHERE id = ?", myid).Scan(&fullname)
	}()
	wg.Wait()

	if email == "" {
		log.Printf("No email found for user %d when creating Stripe subscription", myid)
		return fiber.NewError(fiber.StatusInternalServerError, "Could not retrieve user email")
	}

	name := ""
	if fullname != nil {
		name = *fullname
	}

	log.Printf("Creating Stripe subscription for user %d, amount %d", myid, req.Amount)

	// Create Stripe customer.
	custParams := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
	}
	custParams.AddMetadata("uid", strconv.FormatUint(myid, 10))

	// Protect stripe.Key from concurrent access.
	stripeMu.Lock()
	stripe.Key = key

	cust, err := customer.New(custParams)
	if err != nil {
		stripeMu.Unlock()
		log.Printf("Stripe customer creation failed for user %d: %v", myid, err)
		return fiber.NewError(fiber.StatusInternalServerError, "Payment processing failed")
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
	stripeMu.Unlock()

	if err != nil {
		log.Printf("Stripe subscription creation failed for user %d: %v", myid, err)
		return fiber.NewError(fiber.StatusInternalServerError, "Payment processing failed")
	}

	log.Printf("Stripe subscription created: %s for user %d", sub.ID, myid)

	// Extract client secret from the latest invoice's confirmation secret.
	var clientSecret string
	if sub.LatestInvoice != nil && sub.LatestInvoice.ConfirmationSecret != nil {
		clientSecret = sub.LatestInvoice.ConfirmationSecret.ClientSecret
	}

	return c.JSON(fiber.Map{
		"subscriptionId": sub.ID,
		"clientSecret":   clientSecret,
	})
}
