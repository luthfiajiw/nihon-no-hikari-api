package utils

type SuccessRes struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ErrorRes struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type PageContext struct {
	Page      int `json:"page"`
	TotalPage int `json:"total_page"`
	PageSize  int `json:"page_size"`
}
