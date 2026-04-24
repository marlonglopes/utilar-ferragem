package auth

import "testing"

func TestHashVerifyRoundtrip(t *testing.T) {
	hash, err := HashPassword("utilar123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword("utilar123", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Error("hash não valida a própria senha")
	}
}

func TestVerifyFailsOnWrongPassword(t *testing.T) {
	hash, _ := HashPassword("correto")
	ok, err := VerifyPassword("errado", hash)
	if err != nil {
		t.Fatalf("verify retornou erro: %v", err)
	}
	if ok {
		t.Error("verify passou com senha errada")
	}
}

func TestVerifySeedHash(t *testing.T) {
	// Hash fixo embutido em seed.sql. Se este teste quebrar, o seed não bate
	// com a senha documentada (utilar123).
	seedHash := "$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY"
	ok, err := VerifyPassword("utilar123", seedHash)
	if err != nil {
		t.Fatalf("verify seed hash: %v", err)
	}
	if !ok {
		t.Error("hash do seed não verifica 'utilar123' — seed está inconsistente")
	}
}

func TestDifferentHashesSamePassword(t *testing.T) {
	// salts aleatórios → hashes diferentes, ambos válidos
	h1, _ := HashPassword("x")
	h2, _ := HashPassword("x")
	if h1 == h2 {
		t.Error("dois hashes da mesma senha deveriam ser diferentes (salts aleatórios)")
	}
	ok1, _ := VerifyPassword("x", h1)
	ok2, _ := VerifyPassword("x", h2)
	if !ok1 || !ok2 {
		t.Error("ambos os hashes devem validar")
	}
}

func TestInvalidHashFormat(t *testing.T) {
	_, err := VerifyPassword("x", "not-a-valid-hash")
	if err == nil {
		t.Error("verify não deveria aceitar hash inválido")
	}
}
