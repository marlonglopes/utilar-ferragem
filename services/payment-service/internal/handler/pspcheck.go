package handler

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/utilar/payment-service/internal/psp"
)

// PSPCheck vigia a credencial do gateway de pagamento.
//
// # PORQUÊ existe
//
// A chave do PSP expirou e o sistema não deu nenhum sinal: o /health respondia
// "ok" (só pingava o banco), o boot subia limpo, e a falha só aparecia na
// PRIMEIRA VENDA — como um 502 genérico na cara do cliente, com "payment
// gateway error" no corpo. Ninguém olhando o painel saberia que a loja parou de
// vender.
//
// Credencial inválida é problema de CONFIGURAÇÃO, não de disponibilidade: não
// adianta o cliente tentar de novo, e não vai se resolver sozinho. Precisa
// chegar em quem opera, imediatamente, e não pelo relato do primeiro comprador
// que desistiu.
//
// # PORQUÊ NÃO derruba o boot
//
// Seria tentador recusar subir sem credencial válida. Mas isso amarra o deploy
// à disponibilidade de um terceiro: uma instabilidade momentânea do PSP
// impediria de publicar uma correção urgente em qualquer outra parte do
// sistema. O serviço sobe, atende catálogo e consultas, e grita.
type PSPCheck struct {
	gw       psp.Gateway
	estado   atomic.Value // guarda pspEstado
	interval time.Duration
}

type pspEstado struct {
	OK         bool      `json:"ok"`
	Provider   string    `json:"provider"`
	Motivo     string    `json:"motivo,omitempty"`
	VerifiedAt time.Time `json:"verifiedAt"`
}

// NewPSPCheck cria o vigia. interval zero usa 5 minutos.
func NewPSPCheck(gw psp.Gateway, interval time.Duration) *PSPCheck {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	c := &PSPCheck{gw: gw, interval: interval}
	c.estado.Store(pspEstado{OK: true, Provider: gw.Name(), Motivo: "ainda não verificado"})
	return c
}

// sondaID é um identificador que o PSP nunca terá. A resposta esperada é
// "não encontrado" — e é justamente isso que prova que a credencial foi aceita:
// o gateway só chega a procurar depois de autenticar.
const sondaID = "utilar-credential-probe-0000"

// Verify consulta o PSP uma vez e atualiza o estado.
//
// A leitura do resultado é o ponto sutil: ErrNotFound significa CREDENCIAL BOA
// (o PSP autenticou e só não achou o id). ErrUpstream com 401/403 significa
// credencial ruim. Erro de rede não diz nada sobre a credencial e é tratado
// como inconclusivo — marcar credencial como inválida porque a internet caiu
// geraria alarme falso justamente no momento em que já há barulho demais.
func (c *PSPCheck) Verify(ctx context.Context) pspEstado {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.gw.GetPayment(ctx, sondaID)

	est := pspEstado{Provider: c.gw.Name(), VerifiedAt: time.Now()}
	switch {
	case err == nil, errors.Is(err, psp.ErrNotFound):
		est.OK = true
	case errors.Is(err, psp.ErrInvalidRequest), errors.Is(err, psp.ErrInvalidSignature):
		// O PSP recusou a requisição, mas respondeu — autenticou.
		est.OK = true
	default:
		// Inclui ErrUpstream (401 de chave expirada cai aqui) e falha de rede.
		est.OK = false
		est.Motivo = err.Error()
	}

	c.estado.Store(est)
	if !est.OK {
		slog.Error("psp.credential_check_failed",
			"provider", est.Provider,
			"motivo", est.Motivo,
			"impacto", "toda tentativa de pagamento vai falhar com 502 até a credencial ser corrigida")
	} else {
		slog.Info("psp.credential_ok", "provider", est.Provider)
	}
	return est
}

// Estado devolve a última verificação, para o /health.
func (c *PSPCheck) Estado() pspEstado {
	e, _ := c.estado.Load().(pspEstado)
	return e
}

// Run verifica no boot e depois periodicamente. Chamar em goroutine.
//
// A reverificação existe porque credencial expira COM O SERVIÇO NO AR — foi
// exatamente o caso: a chave venceu sem ninguém tocar em nada.
func (c *PSPCheck) Run(ctx context.Context) {
	c.Verify(ctx)

	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.Verify(ctx)
		}
	}
}
