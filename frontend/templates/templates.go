package templates

import (
	"fmt"
	"html/template"
)

var (
	Main              *template.Template
	Deposit           *template.Template
	Withdraw          *template.Template
	ConfirmDeposit    *template.Template
	ConfirmWithdrawal *template.Template
	Stats             *template.Template
)

func InitTemplates() {
	// Helper function to create a map for passing multiple values to templates
	funcMap := template.FuncMap{
		"dict": func(values ...any) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"safeHTMLAttr": func(s string) template.HTMLAttr {
			return template.HTMLAttr(s)
		},
	}
	tmpl := template.Must(template.New("main").Funcs(funcMap).ParseFiles(
		"frontend/templates/main.html",
		"frontend/templates/confirm_deposit.html",
		"frontend/templates/confirm_withdrawal.html",
		"frontend/templates/stats.html",
	))
	Main = tmpl.Lookup("main")
	Deposit = tmpl.Lookup("depositForm")
	Withdraw = tmpl.Lookup("withdrawForm")
	ConfirmWithdrawal = tmpl.Lookup("confirmWithdrawal")
	ConfirmDeposit = tmpl.Lookup("confirmDeposit")
	Stats = tmpl.Lookup("stats")
}
