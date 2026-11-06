package masters

// This catalog is the allowlist for both SQL identifiers and editable fields.
// No table or column name from an HTTP request is interpolated into SQL.
type Field struct {
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	Type      string   `json:"type"`
	Required  bool     `json:"required"`
	Options   []string `json:"options,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Default   string   `json:"default,omitempty"`
	Max       int      `json:"max,omitempty"`
	Immutable bool     `json:"immutable,omitempty"`
}

type Definition struct {
	Table        string  `json:"-"`
	Title        string  `json:"title"`
	Fields       []Field `json:"fields"`
	Party        bool    `json:"-"`
	NoSoftDelete bool    `json:"-"`
	Key          string  `json:"-"`
}

func text(name, label string, required bool, max int) Field {
	return Field{Name: name, Label: label, Type: "text", Required: required, Max: max}
}
func choice(name, label string, options ...string) Field {
	return Field{Name: name, Label: label, Type: "select", Required: true, Options: options, Default: options[0]}
}
func ref(name, label, kind string, required bool) Field {
	return Field{Name: name, Label: label, Type: "reference", Ref: kind, Required: required}
}
func boolean(name, label, value string) Field {
	return Field{Name: name, Label: label, Type: "boolean", Required: true, Default: value}
}
func date(name, label string, required bool) Field {
	return Field{Name: name, Label: label, Type: "date", Required: required}
}
func number(name, label, kind string, required bool) Field {
	field := Field{Name: name, Label: label, Type: kind, Required: required}
	if name == "version" {
		field.Default = "1"
	}
	if name == "decimal_places" {
		field.Default = "2"
	}
	return field
}
func object(name, label string) Field {
	return Field{Name: name, Label: label, Type: "json", Required: true, Default: "{}"}
}
func common(extra ...Field) []Field {
	return append([]Field{text("code", "Code", true, 100), text("name", "Name", true, 191), boolean("active", "Active", "true")}, extra...)
}
func party(extra ...Field) []Field {
	return append([]Field{ref("company_id", "Company", "company", true), text("code", "Code", true, 100), text("legal_name", "Name", true, 255), boolean("active", "Active", "true"), text("gst_registration", "GST registration", false, 100), text("state_code", "State code", false, 32), text("country_code", "Country code", false, 32), text("email", "Email", false, 254), text("phone", "Phone", false, 64)}, extra...)
}

var catalog = map[string]Definition{
	"sales-types":      {Table: "sales_types", Title: "Sales types", Fields: common(Field{Name: "sales_class", Label: "Classification", Type: "select", Required: true, Options: []string{"international", "domestic", "miscellaneous"}, Default: "international", Immutable: true})},
	"company":          {Table: "company", Title: "Company", NoSoftDelete: true, Fields: []Field{text("legal_name", "Legal name", true, 255), ref("base_currency", "Base currency", "currencies", true)}},
	"branches":         {Table: "branches", Title: "Branches", Fields: append(common(ref("company_id", "Company", "company", true)), text("state_code", "State code", false, 32), text("gst_registration", "GST registration", false, 100))},
	"customers":        {Table: "customers", Title: "Customers", Party: true, Fields: party(number("credit_days", "Credit days", "integer", false), number("credit_limit", "Credit limit", "decimal", false))},
	"agents":           {Table: "travel_agents", Title: "Travel agents", Party: true, Fields: party()},
	"suppliers":        {Table: "suppliers", Title: "Suppliers", Party: true, Fields: party(choice("settlement_channel", "Settlement channel", "airline", "bsp", "consolidator", "other"))},
	"airlines":         {Table: "airlines", Title: "Airlines", Fields: []Field{text("code", "Code", true, 32), text("name", "Name", true, 191), boolean("active", "Active", "true"), text("ticket_prefix", "Ticket prefix", false, 32)}},
	"locations":        {Table: "locations", Title: "Airports & geography", Fields: []Field{choice("kind", "Type", "country", "city", "airport"), text("code", "Code", true, 64), text("name", "Name", true, 191), ref("parent_id", "Parent location", "locations", false), boolean("active", "Active", "true")}},
	"currencies":       {Table: "currencies", Title: "Currencies", Key: "code", Fields: []Field{{Name: "code", Label: "Currency code", Type: "currency", Required: true, Max: 3, Immutable: true}}},
	"exchange-rates":   {Table: "exchange_rates", Title: "Exchange rates", NoSoftDelete: true, Fields: []Field{ref("from_currency", "From currency", "currencies", true), ref("to_currency", "To currency", "currencies", true), date("effective_date", "Effective date", true), number("rate", "Rate", "rate", true)}},
	"accounts":         {Table: "accounts", Title: "Chart of accounts", Fields: common(choice("classification", "Classification", "asset", "liability", "equity", "income", "expense"), ref("parent_id", "Parent account", "accounts", false), boolean("postable", "Allow posting", "true"))},
	"bank-cash":        {Table: "bank_cash_accounts", Title: "Bank & cash accounts", Fields: []Field{ref("ledger_account_id", "Ledger account", "accounts", true), choice("kind", "Type", "bank", "cash"), ref("currency_code", "Currency", "currencies", true), text("bank_name", "Bank name", false, 191), boolean("active", "Active", "true")}},
	"tax":              {Table: "tax_rules", Title: "Tax rules", Fields: []Field{text("code", "Code", true, 100), number("version", "Rule version", "integer", true), text("component", "Component", true, 100), number("rate_percent", "Rate percent", "percent", false), object("basis", "Calculation basis (JSON)"), date("effective_from", "Effective from", true), date("effective_to", "Effective to", false)}},
	"commercial-rules": {Table: "commercial_rules", Title: "Commission, markup & refund rules", Fields: []Field{text("code", "Code", true, 100), choice("kind", "Rule type", "commission", "discount", "markup", "cancellation", "refund", "tds"), object("configuration", "Configuration (JSON)")}},
	"financial-years":  {Table: "financial_years", Title: "Financial years", NoSoftDelete: true, Fields: []Field{text("label", "Name", true, 100), date("starts_on", "Starts on", true), date("ends_on", "Ends on", true), boolean("closed", "Closed", "false")}},
	"periods":          {Table: "accounting_periods", Title: "Accounting periods", NoSoftDelete: true, Fields: []Field{ref("financial_year_id", "Financial year", "financial-years", true), text("label", "Name", true, 100), date("starts_on", "Starts on", true), date("ends_on", "Ends on", true), choice("status", "Status", "open", "closed")}},
}

func init() {
	d := catalog["currencies"]
	d.Fields = append(d.Fields, text("name", "Name", true, 191), number("decimal_places", "Decimal places", "precision", true), boolean("active", "Active", "true"))
	catalog["currencies"] = d
}

var partyColumns = map[string]bool{"company_id": true, "code": true, "legal_name": true, "active": true, "gst_registration": true, "state_code": true, "country_code": true, "email": true, "phone": true}
