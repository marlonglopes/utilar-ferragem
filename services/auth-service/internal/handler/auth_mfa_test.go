package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/utilar/pkg/totp"
)

// Fluxo MFA ponta a ponta: registra → enroll → confirm → login vira 2 passos →
// verify-totp emite os tokens. Exige Postgres :5438 com a migration 007.

func TestMFA_FluxoCompleto(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	email := fmt.Sprintf("mfa-fluxo-%d@utilar.com.br", 1)
	const pass = "secret-pass-1"
	defer db.Exec("DELETE FROM users WHERE email = $1", email)

	// registra e pega o access token
	reg := do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{"email": email, "password": pass, "name": "MFA"})
	if reg.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", reg.Code, reg.Body.String())
	}
	var regResp struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(reg.Body.Bytes(), &regResp)
	token := regResp.AccessToken

	// enroll → segredo + uri
	en := do(r, http.MethodPost, "/api/v1/auth/mfa/enroll", token, nil)
	if en.Code != http.StatusOK {
		t.Fatalf("enroll: %d %s", en.Code, en.Body.String())
	}
	var enResp struct {
		Secret     string `json:"secret"`
		OtpauthURI string `json:"otpauthUri"`
	}
	json.Unmarshal(en.Body.Bytes(), &enResp)
	if enResp.Secret == "" || enResp.OtpauthURI == "" {
		t.Fatalf("enroll sem segredo/uri: %s", en.Body.String())
	}

	// confirm com o código correto → ativa
	code, _ := totp.Code(enResp.Secret)
	cf := do(r, http.MethodPost, "/api/v1/auth/mfa/confirm", token, map[string]any{"code": code})
	if cf.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", cf.Code, cf.Body.String())
	}

	// agora o LOGIN não emite tokens — devolve mfaRequired + challenge
	lg := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{"email": email, "password": pass})
	if lg.Code != http.StatusOK {
		t.Fatalf("login: %d %s", lg.Code, lg.Body.String())
	}
	var lgResp struct {
		MFARequired  bool   `json:"mfaRequired"`
		Challenge    string `json:"challenge"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	json.Unmarshal(lg.Body.Bytes(), &lgResp)
	if !lgResp.MFARequired || lgResp.Challenge == "" {
		t.Fatalf("login com MFA devia pedir 2º fator (challenge), veio: %s", lg.Body.String())
	}
	if lgResp.AccessToken != "" {
		t.Fatal("login com MFA NÃO pode emitir access token no 1º passo")
	}

	// verify-totp com código errado → 401
	if w := do(r, http.MethodPost, "/api/v1/auth/login/verify-totp", "", map[string]any{
		"challenge": lgResp.Challenge, "code": "000000",
	}); w.Code != http.StatusUnauthorized {
		t.Fatalf("código errado devia dar 401, veio %d", w.Code)
	}

	// verify-totp com código certo → tokens
	code2, _ := totp.Code(enResp.Secret)
	vf := do(r, http.MethodPost, "/api/v1/auth/login/verify-totp", "", map[string]any{
		"challenge": lgResp.Challenge, "code": code2,
	})
	if vf.Code != http.StatusOK {
		t.Fatalf("verify-totp: %d %s", vf.Code, vf.Body.String())
	}
	var vfResp struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	json.Unmarshal(vf.Body.Bytes(), &vfResp)
	if vfResp.AccessToken == "" || vfResp.RefreshToken == "" {
		t.Fatalf("verify-totp não emitiu tokens: %s", vf.Body.String())
	}
}

// Challenge inválido/forjado não abre nada.
func TestMFA_ChallengeInvalido(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	if w := do(r, http.MethodPost, "/api/v1/auth/login/verify-totp", "", map[string]any{
		"challenge": "nao.e.um.jwt.valido", "code": "123456",
	}); w.Code != http.StatusUnauthorized {
		t.Fatalf("challenge forjado devia dar 401, veio %d", w.Code)
	}
}

// Sem MFA ativo, o login continua emitindo tokens direto (não quebrou o fluxo).
func TestMFA_LoginSemMFAContinuaDireto(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "test1@utilar.com.br", "password": "utilar123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		MFARequired bool   `json:"mfaRequired"`
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.MFARequired || resp.AccessToken == "" {
		t.Fatalf("conta sem MFA devia logar direto com token, veio: %s", w.Body.String())
	}
}
