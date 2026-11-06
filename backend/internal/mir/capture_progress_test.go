package mir

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"voltafinbook/backend/internal/mir/decoder"
)

func TestImportedBoundsRespectBusinessMidnightAndDST(t *testing.T) {
	for _, tc := range []struct {
		zone, date, from, to string
		hours                time.Duration
	}{
		{"UTC", "2026-11-06", "2026-11-06T00:00:00Z", "2026-11-07T00:00:00Z", 24},
		{"Asia/Kolkata", "2026-11-06", "2026-11-05T18:30:00Z", "2026-11-06T18:30:00Z", 24},
		{"America/New_York", "2026-03-08", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z", 23},
		{"America/New_York", "2026-11-01", "2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z", 25},
	} {
		t.Run(tc.zone+tc.date, func(t *testing.T) {
			location, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			first, last, err := importedBounds(tc.date, tc.date, location)
			if err != nil || first.Format(time.RFC3339) != tc.from || last.Format(time.RFC3339) != tc.to || last.Sub(first) != tc.hours*time.Hour {
				t.Fatalf("wrong local-day boundaries: %s %s %v", first, last, err)
			}
		})
	}
	if _, _, err := importedBounds("2026-11-07", "2026-11-06", time.UTC); err == nil {
		t.Fatal("inverted date range accepted")
	}
}
func TestCaptureSettingsDoesNotGuessTimezone(t *testing.T) {
	h := StageHandler{}
	for _, zone := range []string{"", "Asia/Kolkata", "not/a-timezone"} {
		t.Setenv("BUSINESS_TIMEZONE", zone)
		response := httptest.NewRecorder()
		h.captureSettings(response, httptest.NewRequest("GET", "/api/v1/mir/capture/settings", nil))
		if zone == "not/a-timezone" {
			if response.Code != 503 {
				t.Fatal("invalid timezone silently accepted")
			}
		} else if response.Code != 200 || !strings.Contains(response.Body.String(), "timezone") {
			t.Fatal(response.Body.String())
		}
	}
}
func TestProgressUsesRealValidationAndMissingSourceFields(t *testing.T) {
	complete := CaptureRow{TicketNumber: "1231234567890", PassengerName: "TEST/USER", PNR: "TEST01", IssueDate: "2026-11-06", Sector: "AAA-BBB", Fare: "120.00", Currency: "USD"}
	ready := sourceProgress(complete, "staged", nil)
	if ready.Status != "Ready for Sale" || len(ready.Issues) != 0 || !ready.CanPrepareDraft {
		t.Fatalf("complete source not ready: %#v", ready)
	}
	incomplete := complete
	incomplete.PassengerName = ""
	incomplete.Fare = "USD 120.00"
	review := sourceProgress(incomplete, "staged", nil)
	if review.Status != "Needs Review" || len(review.Issues) != 2 {
		t.Fatalf("missing/invalid source ignored: %#v", review)
	}
	failed := sourceProgress(complete, "failed", nil)
	if failed.Status != "Needs Review" || failed.CanPrepareDraft {
		t.Fatal("failed extraction can prepare a draft")
	}
	invalid := sourceProgress(complete, "review", []decoder.Issue{{Severity: "error", Message: "Passenger fare link is ambiguous."}})
	if invalid.Status != "Needs Review" || len(invalid.Issues) != 1 {
		t.Fatal("stored validation issue ignored")
	}
	if ticketKey("123-1234567890") != ticketKey("1231234567890") {
		t.Fatal("ticket identity normalization differs")
	}
}
