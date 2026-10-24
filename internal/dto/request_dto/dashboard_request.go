package requestdto

type DashboardRequest struct {
	DateStart *string `query:"date_start,omitempty"`
	DateEnd   *string `query:"date_end,omitempty"`
}
