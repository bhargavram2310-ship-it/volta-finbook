package employees

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserInputValidation(t *testing.T) {
	valid := func() userInput {
		return userInput{
			EmployeeID: "A7E658FBB3D248F4A3EF1FE09AF17010", Username: " staff ",
			LoginMethod: "username", DisplayName: " Staff ", AccountStatus: "Active", Password: "Temporary-test-123",
			Remarks: " New hire ", RoleIDs: []string{},
		}
	}
	input := valid()
	if !validUser(&input, false) || input.Username != "staff" || input.EmployeeID != "a7e658fb-b3d2-48f4-a3ef-1fe09af17010" {
		t.Fatal("valid user was not normalized")
	}
	input = valid()
	input.Password = "six789"
	if !validUser(&input, false) {
		t.Fatal("six-character password was rejected")
	}
	input = valid()
	input.LoginMethod = "email"
	input.LoginEmail = "staff@example.invalid"
	if !validUser(&input, false) {
		t.Fatal("valid email login rejected")
	}
	for name, mutate := range map[string]func(*userInput){
		"missing employee":     func(input *userInput) { input.EmployeeID = "" },
		"ambiguous username":   func(input *userInput) { input.Username = "staff@example.invalid" },
		"invalid email":        func(input *userInput) { input.LoginEmail = "invalid" },
		"short password":       func(input *userInput) { input.Password = "short" },
		"bcrypt byte limit":    func(input *userInput) { input.Password = strings.Repeat("é", 37) },
		"invalid status":       func(input *userInput) { input.AccountStatus = "unknown" },
		"missing role list":    func(input *userInput) { input.RoleIDs = nil },
		"invalid login method": func(input *userInput) { input.LoginMethod = "phone" },
		"email login requires email": func(input *userInput) {
			input.LoginMethod = "email"
			input.LoginEmail = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := valid()
			mutate(&input)
			if validUser(&input, false) {
				t.Fatal("invalid user accepted")
			}
		})
	}
	input = valid()
	input.Version = 1
	if validUser(&input, true) {
		t.Fatal("password changes must use the password endpoint")
	}
	input.Password = ""
	if !validUser(&input, true) {
		t.Fatal("valid update rejected")
	}
	input.Version = 0
	if validUser(&input, true) {
		t.Fatal("unversioned update accepted")
	}
}

func TestUserAndRoleRoutesRequireSession(t *testing.T) {
	handler := NewHandler(nil, false)
	for _, path := range []string{"/api/v1/users", "/api/v1/users/options", "/api/v1/roles"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s returned %d, want unauthorized", path, response.Code)
		}
	}
}

func TestRoleInputValidation(t *testing.T) {
	input := roleInput{Code: " OPS_ADMIN ", Name: " Operations Admin ", Active: true}
	if !validRoleInput(&input, false) || input.Code != "OPS_ADMIN" || input.Name != "Operations Admin" {
		t.Fatal("valid role was not normalized")
	}
	for _, invalid := range []roleInput{
		{Code: "", Name: "Role"},
		{Code: "INVALID CODE", Name: "Role"},
		{Code: "OPS", Name: ""},
		{Code: strings.Repeat("x", 101), Name: "Role"},
		{Code: "OPS", Name: "Role", Version: 1},
	} {
		if validRoleInput(&invalid, false) {
			t.Errorf("invalid role was accepted: %#v", invalid)
		}
	}
	input.Version = 1
	if !validRoleInput(&input, true) {
		t.Fatal("valid role update was rejected")
	}
}

func TestRoleIDNormalization(t *testing.T) {
	id := "A7E658FBB3D248F4A3EF1FE09AF17010"
	roles, err := normalizeRoleIDs([]string{id})
	if err != nil || len(roles) != 1 || roles[0] != "a7e658fb-b3d2-48f4-a3ef-1fe09af17010" {
		t.Fatalf("valid role ID was not normalized: %#v, %v", roles, err)
	}
	for _, values := range [][]string{nil, {"bad"}, {id, id}} {
		if _, err := normalizeRoleIDs(values); err == nil {
			t.Errorf("invalid role ID list was accepted: %#v", values)
		}
	}
}

func TestAccessRequestRejectsCrossOriginAndMalformedJSON(t *testing.T) {
	for _, testCase := range []struct {
		body, origin string
		status       int
	}{
		{`{}`, "https://foreign.invalid", http.StatusForbidden},
		{`{} {}`, "", http.StatusBadRequest},
		{`{"unknown":true}`, "", http.StatusBadRequest},
		{`{}`, "http://example.com", http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://example.com/api/v1/users", strings.NewReader(testCase.body))
		if testCase.origin != "" {
			request.Header.Set("Origin", testCase.origin)
		}
		response := httptest.NewRecorder()
		var input userInput
		ok := decodeAccess(response, request, &input)
		if testCase.status == http.StatusOK {
			if !ok {
				t.Fatal("same origin request rejected")
			}
		} else if ok || response.Code != testCase.status {
			t.Fatalf("unexpected request acceptance: %d", response.Code)
		}
	}
}
