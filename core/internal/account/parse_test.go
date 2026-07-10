package account

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

const testJWT = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.abcdefgh"

func TestParseImportDocumentUTF8BOM(t *testing.T) {
	raw := append([]byte{0xef, 0xbb, 0xbf}, []byte(`[{"access_token":"`+testJWT+`"},{"refreshToken":"refresh_12345678901234567890"}]`)...)
	entries, err := ParseImportDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Entry.AccessToken != testJWT || entries[1].Entry.RefreshToken == "" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestParseImportDocumentUTF16(t *testing.T) {
	text := `[{"access_token":"` + testJWT + `"}]`
	for _, bigEndian := range []bool{false, true} {
		units := utf16.Encode([]rune(text))
		raw := []byte{0xff, 0xfe}
		var order binary.ByteOrder = binary.LittleEndian
		if bigEndian {
			raw = []byte{0xfe, 0xff}
			order = binary.BigEndian
		}
		for _, unit := range units {
			pair := make([]byte, 2)
			order.PutUint16(pair, unit)
			raw = append(raw, pair...)
		}
		entries, err := ParseImportDocument(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Entry.AccessToken != testJWT {
			t.Fatalf("unexpected UTF-16 entries: %#v", entries)
		}
	}
}

func TestParseImportDocumentRejectsMalformedJSONAndOrdinaryText(t *testing.T) {
	if _, err := ParseImportDocument([]byte(`[{"access_token":`)); err == nil {
		t.Fatal("malformed JSON was accepted")
	}
	entries, err := ParseImportDocument([]byte("this is a heading"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Err == nil {
		t.Fatalf("ordinary text was not marked invalid: %#v", entries)
	}
}

func TestParseImportDocumentRejectsOversize(t *testing.T) {
	if _, err := ParseImportDocument(make([]byte, MaxImportBytes+1)); err == nil {
		t.Fatal("oversized import was accepted")
	}
}

func TestParseImportDocumentIgnoresAmbiguousAccountID(t *testing.T) {
	entries, err := ParseImportDocument([]byte(`{"account_id":"shared","access_token":"` + testJWT + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Entry.ChatGPTAccountID != "" {
		t.Fatalf("ambiguous account_id became ChatGPT ID: %#v", entries[0])
	}
	if len(entries[0].Warnings) == 0 {
		t.Fatal("ambiguous account_id did not produce a warning")
	}
}

func TestParseImportDocumentJSONStreamAndBareTokens(t *testing.T) {
	raw := `{"access_token":"` + testJWT + `"}` + "\n" + `{"refresh_token":"refresh_12345678901234567890"}`
	entries, err := ParseImportDocument([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("JSON stream entries = %d, want 2", len(entries))
	}
	bare, err := ParseImportDocument([]byte(strings.Join([]string{testJWT, "refresh_12345678901234567890"}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(bare) != 2 || bare[1].Entry.RefreshToken == "" {
		t.Fatalf("bare token entries unexpected: %#v", bare)
	}
}
