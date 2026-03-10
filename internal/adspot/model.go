package adspot

// Placement values.
const (
	PlacementHomeScreen  = "home_screen"
	PlacementRideSummary = "ride_summary"
	PlacementMapView     = "map_view"
)

// Status values.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// AdSpot represents a single ad placement.
type AdSpot struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	ImageURL      string  `json:"imageUrl"`
	Placement     string  `json:"placement"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"createdAt"`
	DeactivatedAt *string `json:"deactivatedAt,omitempty"`
	TTLMinutes    *int    `json:"ttlMinutes,omitempty"`
}

// CreateRequest is the payload for POST /adspots.
type CreateRequest struct {
	Title      string `json:"title"`
	ImageURL   string `json:"imageUrl"`
	Placement  string `json:"placement"`
	TTLMinutes *int   `json:"ttlMinutes,omitempty"`
}

func validPlacement(p string) bool {
	switch p {
	case PlacementHomeScreen, PlacementRideSummary, PlacementMapView:
		return true
	}
	return false
}
