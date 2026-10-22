package schemas

type TutorCode struct {
	Code   string `json:"code" pattern:"^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$"`
	Active bool   `json:"active"`
}
