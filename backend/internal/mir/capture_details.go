package mir

import (
	"database/sql"
	"errors"
	"net/http"
	"voltafinbook/backend/internal/auth"
)

func (h *StageHandler) captureDetails(w http.ResponseWriter, r *http.Request) {
	passengerID, err := parseID(r.PathValue("id"))
	if err != nil {
		failure(w, 400, "Invalid passenger ID.")
		return
	}
	recordID, err := parseID(r.URL.Query().Get("record_id"))
	if err != nil || len(r.URL.Query()["record_id"]) != 1 {
		failure(w, 400, "Invalid record ID.")
		return
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	row, err := scanCapture(repo.DB.QueryRowContext(ctx, "SELECT "+captureColumns+captureJoin+" WHERE p.id=? AND r.id=?", passengerID, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		failure(w, 404, "Ticket no longer available. Search again.")
		return
	}
	if err != nil {
		failure(w, 503, "Ticket details could not be loaded. Try again.")
		return
	}
	records := []CaptureRow{row}
	if err := repo.enrichCapture(ctx, records); err != nil {
		failure(w, 503, "Ticket validation and sale links could not be checked. Try again.")
		return
	}
	actor, _ := auth.PrincipalFromContext(r.Context())
	allowed, err := repo.canViewSales(ctx, actor.ID)
	if err != nil {
		failure(w, 503, "Sales permissions could not be checked. Try again.")
		return
	}
	records[0].Progress.CanViewSale = allowed
	draft, draftErr := repo.captureDraft(ctx, recordID, passengerID, true)
	type tax struct {
		Code     string `json:"code"`
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	}
	taxes := []tax{}
	rows, err := repo.DB.QueryContext(ctx, `SELECT tax.tax_code,tax.amount,tax.currency FROM mir_fare_tax tax WHERE tax.record_id=? AND tax.source_item_id IN (SELECT link.to_item_id FROM mir_item_relationship link JOIN mir_passenger passenger ON passenger.source_item_id=link.from_item_id WHERE passenger.id=? AND passenger.record_id=? AND link.relationship_type='passenger_fare') ORDER BY tax.id`, recordID, passengerID, recordID)
	if err != nil {
		failure(w, 503, "Ticket tax details could not be loaded. Try again.")
		return
	}
	for rows.Next() {
		var t tax
		if err = rows.Scan(&t.Code, &t.Amount, &t.Currency); err != nil {
			break
		}
		taxes = append(taxes, t)
	}
	rowsErr := rows.Err()
	rows.Close()
	if err != nil || rowsErr != nil {
		failure(w, 503, "Ticket tax details could not be read. Try again.")
		return
	}
	body := map[string]any{"ticket": records[0], "taxes": taxes, "draft": nil, "draft_error": "", "customer_mapping": "Choose a customer in Entry. The source client reference has no verified customer-master mapping.", "accounting_note": "Imported fare is source evidence. Review customer, supplier and sale amounts in Entry before saving."}
	if sale := records[0].Progress.LinkedSale; sale != nil {
		if allowed {
			var customer string
			err := repo.DB.QueryRowContext(ctx, `SELECT COALESCE(p.legal_name,'') FROM sales_documents d LEFT JOIN customers c ON c.id=d.customer_id LEFT JOIN parties p ON p.id=c.party_id WHERE d.id=UNHEX(?)`, sale.ID).Scan(&customer)
			if err != nil {
				failure(w, 503, "Customer mapping could not be read. Try again.")
				return
			}
			if customer == "" {
				customer = "No customer is mapped on the linked sale."
			}
			body["customer_mapping"] = customer
		} else {
			body["customer_mapping"] = "Customer mapping is on the linked sale. Your role does not have permission to view sales records."
		}
	}
	if draftErr == nil {
		body["draft"] = draft
	} else {
		body["draft_error"] = "The source needs review in MIR Import Report before a draft can be prepared."
		records[0].Progress.CanPrepareDraft = false
		body["ticket"] = records[0]
	}
	respond(w, 200, body)
}
