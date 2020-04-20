package product

import (
	"html/template"
	"time"
)

// Product is an item we sell.
type Product struct {
	ID          string    `json:"id"`           // Unique identifier.
	Name        string    `json:"name"`         // Display name of the product.
	Cost        int       `json:"cost"`         // Price for one item in cents.
	Quantity    int       `json:"quantity"`     // Original number of items available.
	Sold        int       `json:"sold"`         // Aggregate field showing number of items sold.
	Revenue     int       `json:"revenue"`      // Aggregate field showing total cost of sold items.
	UserID      string    `json:"user_id"`      // ID of the user who created the product.
	DateCreated time.Time `json:"date_created"` // When the product was added.
	DateUpdated time.Time `json:"date_updated"` // When the product record was last modified.
}

// NameHTML fixes encoding issues.
func (p *Product) NameHTML() template.HTML {
	return template.HTML(p.Name)
}
