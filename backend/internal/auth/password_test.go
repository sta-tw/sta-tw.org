package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	valid, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !valid {
		t.Fatalf("VerifyPassword(valid) = %v, %v", valid, err)
	}
	valid, err = VerifyPassword("wrong password", hash)
	if err != nil || valid {
		t.Fatalf("VerifyPassword(invalid) = %v, %v", valid, err)
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("password", "not-a-password-hash"); err == nil {
		t.Fatal("VerifyPassword() error = nil, want malformed hash error")
	}
}
