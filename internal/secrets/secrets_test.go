package secrets

import "testing"

const key = "6d696e692d76657263656c2d6465762d6b65792d33322d62797465732d212121"

func TestSealOpenRoundTrip(t *testing.T) {
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	ct, nonce, err := b.Seal("s3cr3t", "prj_1/API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Open(ct, nonce, "prj_1/API_KEY")
	if err != nil || got != "s3cr3t" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestCiphertextBoundToContext(t *testing.T) {
	b, _ := New(key)
	ct, nonce, _ := b.Seal("s3cr3t", "prj_1/API_KEY")
	if _, err := b.Open(ct, nonce, "prj_2/API_KEY"); err == nil {
		t.Fatal("expected decryption with a different project to fail")
	}
}

func TestRejectsShortKey(t *testing.T) {
	if _, err := New("abcd"); err == nil {
		t.Fatal("expected error for short key")
	}
}
