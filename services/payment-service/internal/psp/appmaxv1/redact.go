package appmaxv1

import (
	"fmt"
	"log/slog"
)

// Proteção do token OAuth2 e das credenciais contra vazamento acidental.
//
// O RISCO CONCRETO (audit AV1-H4): `Client` guarda o access token e o
// client_secret em memória. Basta UM `slog.Error("appmax falhou", "client", c)`,
// um `fmt.Errorf("%+v", cfg)` num handler de erro, ou um panic com a struct no
// stack dump, pra que a credencial de produção vá parar no agregador de logs —
// que tem retenção longa e acesso mais amplo que o cofre de segredos.
//
// A defesa é fazer com que a forma NATURAL de imprimir a struct já seja segura:
// implementamos slog.LogValuer, fmt.Stringer e fmt.GoStringer. Assim `%v`,
// `%+v`, `%#v` e slog produzem a versão mascarada, e não existe caminho fácil
// de imprimir o segredo por engano.

// mask mostra só o suficiente pra correlacionar dois logs ("é a mesma
// credencial?") sem entregar o valor. Segredo curto vira "***" inteiro:
// mostrar 4 de 8 caracteres é entregar metade da chave.
func mask(s string) string {
	switch {
	case s == "":
		return ""
	case len(s) <= 12:
		return "***"
	default:
		return s[:4] + "***" + s[len(s)-2:]
	}
}

// LogValue implementa slog.LogValuer — `slog.Any("cfg", cfg)` sai mascarado.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("auth_url", c.AuthURL),
		slog.String("api_url", c.APIURL),
		slog.String("client_id", mask(c.ClientID)),
		slog.String("client_secret", mask(c.ClientSecret)),
		slog.String("external_id", c.ExternalID),
		slog.Bool("webhook_secret_set", c.WebhookSecret != ""),
		slog.Duration("http_timeout", c.HTTPTimeout),
	)
}

// String/GoString cobrem fmt: `%v`, `%s` e `%#v` da Config nunca imprimem
// segredo, nem quando embutida em outra struct.
func (c Config) String() string {
	return fmt.Sprintf("appmaxv1.Config{auth=%s api=%s client_id=%s client_secret=%s webhook_secret_set=%t}",
		c.AuthURL, c.APIURL, mask(c.ClientID), mask(c.ClientSecret), c.WebhookSecret != "")
}

func (c Config) GoString() string { return c.String() }

// LogValue do Client: nunca expõe o access token, só se existe e até quando
// vale — que é a informação de que se precisa pra debugar "por que deu 401".
func (c *Client) LogValue() slog.Value {
	c.mu.RLock()
	hasToken := c.token != ""
	exp := c.tokenExpiry
	c.mu.RUnlock()
	return slog.GroupValue(
		slog.String("provider", ProviderName),
		slog.String("api_url", c.cfg.APIURL),
		slog.Bool("token_cached", hasToken),
		slog.Time("token_expires_at", exp),
	)
}

func (c *Client) String() string {
	c.mu.RLock()
	hasToken := c.token != ""
	c.mu.RUnlock()
	return fmt.Sprintf("appmaxv1.Client{api=%s token_cached=%t}", c.cfg.APIURL, hasToken)
}

func (c *Client) GoString() string { return c.String() }

// LogValue/String do Gateway — o webhookSecret também é credencial.
func (g *Gateway) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("provider", ProviderName),
		slog.Bool("webhook_secret_set", g.webhookSecret != ""),
	)
}

func (g *Gateway) String() string {
	return fmt.Sprintf("appmaxv1.Gateway{provider=%s webhook_secret_set=%t}", ProviderName, g.webhookSecret != "")
}

func (g *Gateway) GoString() string { return g.String() }

// Asserções de compile-time: se alguém remover um destes métodos, o build quebra
// antes de a credencial voltar a ser imprimível.
var (
	_ slog.LogValuer = Config{}
	_ fmt.Stringer   = Config{}
	_ fmt.GoStringer = Config{}
	_ slog.LogValuer = (*Client)(nil)
	_ fmt.Stringer   = (*Client)(nil)
	_ fmt.GoStringer = (*Client)(nil)
	_ slog.LogValuer = (*Gateway)(nil)
	_ fmt.Stringer   = (*Gateway)(nil)
	_ fmt.GoStringer = (*Gateway)(nil)
)
