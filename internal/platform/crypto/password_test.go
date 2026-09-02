package crypto

import "testing"

func testParams() PasswordParams {
	return PasswordParams{MemoryKiB: 65536, Iterations: 3, Parallelism: 2}
}

func TestHashAndVerify(t *testing.T) {
	h, err := NewPasswordHasher(testParams())
	if err != nil {
		t.Fatalf("NewPasswordHasher: %v", err)
	}
	encoded, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	ok, err := Verify("correct horse battery staple", encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("Verify returned false for the correct password")
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	h, _ := NewPasswordHasher(testParams())
	encoded, _ := h.Hash("correct horse battery staple")
	ok, err := Verify("wrong password", encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Fatal("Verify returned true for an incorrect password")
	}
}

func TestHashIsSalted(t *testing.T) {
	h, _ := NewPasswordHasher(testParams())
	a, _ := h.Hash("same password")
	b, _ := h.Hash("same password")
	if a == b {
		t.Fatal("two hashes of the same password with fresh salts should differ")
	}
}

func TestHashEmptyPasswordRejected(t *testing.T) {
	h, _ := NewPasswordHasher(testParams())
	if _, err := h.Hash(""); err == nil {
		t.Fatal("expected error hashing an empty password")
	}
}

func TestNewPasswordHasherEnforcesFloor(t *testing.T) {
	cases := []PasswordParams{
		{MemoryKiB: 1024, Iterations: 3, Parallelism: 2},  // below 19 MiB
		{MemoryKiB: 65536, Iterations: 1, Parallelism: 2}, // below 2 iterations
		{MemoryKiB: 65536, Iterations: 3, Parallelism: 0}, // below 1 parallelism
	}
	for _, c := range cases {
		if _, err := NewPasswordHasher(c); err == nil {
			t.Errorf("expected floor violation error for %+v, got nil", c)
		}
	}
}

func TestVerifyMalformedHash(t *testing.T) {
	if _, err := Verify("anything", "not-a-real-hash"); err == nil {
		t.Fatal("expected error for malformed encoded hash")
	}
}

func TestVerifyToleratesParameterChange(t *testing.T) {
	// A hash produced at one cost must still verify correctly even if the
	// process's *current* default params later change, since the params
	// are embedded in the stored string.
	oldHasher, _ := NewPasswordHasher(PasswordParams{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1})
	encoded, _ := oldHasher.Hash("legacy user password")

	ok, err := Verify("legacy user password", encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("Verify should succeed using the hash's own embedded params, not the caller's current defaults")
	}
}

func BenchmarkHash_Floor_19MiB_t2_p1(b *testing.B) {
	h, _ := NewPasswordHasher(PasswordParams{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash("benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHash_64MiB_t3_p2(b *testing.B) {
	h, _ := NewPasswordHasher(PasswordParams{MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 2})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash("benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHash_64MiB_t4_p4(b *testing.B) {
	h, _ := NewPasswordHasher(PasswordParams{MemoryKiB: 64 * 1024, Iterations: 4, Parallelism: 4})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash("benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHash_128MiB_t3_p2(b *testing.B) {
	h, _ := NewPasswordHasher(PasswordParams{MemoryKiB: 128 * 1024, Iterations: 3, Parallelism: 2})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash("benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHash_256MiB_t2_p2(b *testing.B) {
	h, _ := NewPasswordHasher(PasswordParams{MemoryKiB: 256 * 1024, Iterations: 2, Parallelism: 2})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash("benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHash_192MiB_t2_p2(b *testing.B) {
	h, _ := NewPasswordHasher(PasswordParams{MemoryKiB: 192 * 1024, Iterations: 2, Parallelism: 2})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash("benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}
