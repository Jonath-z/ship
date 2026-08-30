package environmentvariables

import "testing"

func TestParseDotenv(t *testing.T) {
	entries, err := parseDotenv("# comment\nNODE_ENV=production\nexport API_URL=\"https://example.com\\npath\"\nTOKEN='literal value'\nEMPTY=\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 || entries[0].Name != "NODE_ENV" || entries[0].Value != "production" ||
		entries[1].Value != "https://example.com\npath" || entries[2].Value != "literal value" || entries[3].Value != "" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestParseDotenvRejectsDuplicates(t *testing.T) {
	if _, err := parseDotenv("PORT=3000\nPORT=4000\n"); err == nil {
		t.Fatal("expected duplicate name to be rejected")
	}
}

func TestSecretValidationRejectsEmptyValue(t *testing.T) {
	if err := validateNameAndValue("DATABASE_URL", "", true); err == nil {
		t.Fatal("expected empty secret value to be rejected")
	}
}
