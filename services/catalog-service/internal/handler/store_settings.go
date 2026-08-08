// Configuração da loja — o aviso da vitrine que o dono edita sem deploy.
//
// GET é PÚBLICO (a vitrine lê pra desenhar o banner) e nunca devolve dado
// sensível — só o aviso. PUT é admin-only (é a comunicação oficial da loja com o
// cliente; deixar `vendas` mudar o banner da home é dar voz institucional a quem
// mantém o catálogo). Ver docs/backoffice-personas.md.
//
// Singleton: sempre id=1 (ver migration 020). Não há "criar" nem "listar" — a
// linha já existe semeada e desligada; o dono só liga/edita.
package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// maxAnnouncementLen — teto do texto do aviso. Banner, não parágrafo: o que não
// couber numa linha o cliente não lê, e um texto gigante quebraria o layout.
const maxAnnouncementLen = 280

// announcementLevels — tons aceitos. Espelha o CHECK do banco; validamos aqui pra
// devolver 400 com mensagem, em vez de deixar o INSERT estourar erro cru.
var announcementLevels = map[string]bool{"info": true, "warning": true, "success": true}

type StoreSettingsHandler struct {
	db *sql.DB
}

func NewStoreSettingsHandler(db *sql.DB) *StoreSettingsHandler {
	return &StoreSettingsHandler{db: db}
}

type announcement struct {
	Enabled bool   `json:"enabled"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

type storeSettings struct {
	Announcement announcement `json:"announcement"`
}

// Get GET /api/v1/store/settings — público. A vitrine chama pra saber se há aviso.
func (h *StoreSettingsHandler) Get(c *gin.Context) {
	var s storeSettings
	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT announcement_enabled, announcement_message, announcement_level
		FROM store_settings WHERE id = 1`).
		Scan(&s.Announcement.Enabled, &s.Announcement.Message, &s.Announcement.Level)
	// Linha ausente não deveria acontecer (a migration semeia), mas se acontecer
	// devolvemos o default desligado — a vitrine nunca deve quebrar por causa do
	// banner.
	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, storeSettings{Announcement: announcement{Level: "info"}})
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, s)
}

// Update PUT /api/v1/admin/store/settings — admin-only.
func (h *StoreSettingsHandler) Update(c *gin.Context) {
	var req announcement
	// O corpo é { "enabled": ..., "message": ..., "level": ... } (o próprio
	// announcement — não aninhamos no PUT pra não obrigar o cliente a mandar um
	// envelope com um campo só).
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if len(req.Message) > maxAnnouncementLen {
		BadRequest(c, "o aviso é longo demais (máx 280 caracteres)")
		return
	}
	if req.Level == "" {
		req.Level = "info"
	}
	if !announcementLevels[req.Level] {
		BadRequest(c, "tom inválido (use info, warning ou success)")
		return
	}
	// Ligar o aviso com a mensagem vazia mostraria uma barra em branco na vitrine
	// — provavelmente engano do dono. Barramos com mensagem acionável.
	if req.Enabled && req.Message == "" {
		BadRequest(c, "escreva a mensagem antes de ligar o aviso")
		return
	}

	updatedBy := c.GetString("user_id")
	_, err := h.db.ExecContext(c.Request.Context(), `
		UPDATE store_settings SET
			announcement_enabled = $1,
			announcement_message = $2,
			announcement_level   = $3,
			updated_at           = now(),
			updated_by           = $4
		WHERE id = 1`,
		req.Enabled, req.Message, req.Level, updatedBy)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, storeSettings{Announcement: req})
}
