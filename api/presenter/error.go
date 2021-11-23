package presenter

type ErrorsResponse struct {
	Errors []PresentedError `json:"errors"`
}

type PresentedError struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
	Code   int    `json:"code"`
}
