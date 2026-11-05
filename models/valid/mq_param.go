package valid

type PushParam struct {
	URL    string `json:"url"`
	Body   string `json:"body" binding:"required"`
	Header string `json:"header" binding:"required"`
}
