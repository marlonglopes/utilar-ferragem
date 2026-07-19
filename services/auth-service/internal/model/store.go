package model

import "time"

// ============================================================================
// PDV de balcão — lojas, operadores e clientes de balcão
// ============================================================================

// Papéis e cargos.
//
// RoleStoreOperator é o PAPEL (superfície: pode abrir o PDV). O cargo abaixo é
// o NÍVEL dentro dessa superfície (quanto desconto pode dar sem aprovação).
// Ver o comentário longo em migrations/004_store_operators.up.sql.
const (
	RoleCustomer      = "customer"
	RoleSeller        = "seller"
	RoleAdmin         = "admin"
	RoleStoreOperator = "store_operator"
)

// StoreLevel — cargo do operador dentro da loja.
type StoreLevel string

const (
	LevelOperator   StoreLevel = "operator"
	LevelSupervisor StoreLevel = "supervisor"
	LevelManager    StoreLevel = "manager"
)

// ValidStoreLevel evita que um PATCH mande um cargo inventado e só descubra no
// erro do enum do Postgres (que vaza schema na mensagem).
func ValidStoreLevel(l StoreLevel) bool {
	switch l {
	case LevelOperator, LevelSupervisor, LevelManager:
		return true
	}
	return false
}

type Store struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	CNPJ         string    `json:"cnpj"`
	Street       string    `json:"street"`
	Number       string    `json:"number"`
	Complement   *string   `json:"complement,omitempty"`
	Neighborhood string    `json:"neighborhood"`
	City         string    `json:"city"`
	State        string    `json:"state"`
	CEP          string    `json:"cep"`
	Phone        *string   `json:"phone,omitempty"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"createdAt"`
}

// StoreOperator é a visão que o PDV e o order-service consomem.
//
// DiscountCeilingPct já vem RESOLVIDO (override individual, se houver, senão o
// teto do cargo). Quem consome não precisa saber que existe override — e não
// pode errar a precedência.
type StoreOperator struct {
	UserID             string     `json:"userId"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	StoreID            string     `json:"storeId"`
	StoreCode          string     `json:"storeCode"`
	StoreName          string     `json:"storeName"`
	Level              StoreLevel `json:"level"`
	DiscountCeilingPct float64    `json:"discountCeilingPct"`
	CanApproveDiscount bool       `json:"canApproveDiscount"`
	Active             bool       `json:"active"`
	CreatedAt          time.Time  `json:"createdAt"`
}

// StoreCustomer — cadastro leve do cliente de balcão (sem senha, sem e-mail
// obrigatório). Ver LGPD em migrations/004.
type StoreCustomer struct {
	ID           string    `json:"id"`
	Document     string    `json:"document"`
	DocumentType string    `json:"documentType"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	Email        *string   `json:"email,omitempty"`
	Segment      string    `json:"segment"`
	UserID       *string   `json:"userId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// -- Request payloads -------------------------------------------------------

type StoreRequest struct {
	Code         string  `json:"code" binding:"required,min=2,max=32"`
	Name         string  `json:"name" binding:"required,min=2,max=120"`
	CNPJ         string  `json:"cnpj" binding:"required,min=14,max=18"`
	Street       string  `json:"street" binding:"required,max=255"`
	Number       string  `json:"number" binding:"required,max=20"`
	Complement   *string `json:"complement,omitempty" binding:"omitempty,max=100"`
	Neighborhood string  `json:"neighborhood" binding:"required,max=100"`
	City         string  `json:"city" binding:"required,max=100"`
	State        string  `json:"state" binding:"required,len=2"`
	CEP          string  `json:"cep" binding:"required,max=9"`
	Phone        *string `json:"phone,omitempty" binding:"omitempty,max=20"`
}

// CreateOperatorRequest — promove um usuário existente a operador de balcão.
//
// Por que "usuário existente" e não criar do zero: a pessoa já tem identidade
// (e-mail + senha + reset de senha + auditoria de login). Um segundo caminho de
// criação de credencial seria uma segunda superfície de ataque para manter.
type CreateOperatorRequest struct {
	UserID  string     `json:"userId" binding:"required,uuid"`
	StoreID string     `json:"storeId" binding:"required,uuid"`
	Level   StoreLevel `json:"level" binding:"required,oneof=operator supervisor manager"`
	// Override individual do teto. Ausente = usa o teto do cargo.
	DiscountCeilingPct *float64 `json:"discountCeilingPct,omitempty" binding:"omitempty,gte=0,lte=100"`
}

type UpdateOperatorRequest struct {
	StoreID            *string     `json:"storeId,omitempty" binding:"omitempty,uuid"`
	Level              *StoreLevel `json:"level,omitempty" binding:"omitempty,oneof=operator supervisor manager"`
	DiscountCeilingPct *float64    `json:"discountCeilingPct,omitempty" binding:"omitempty,gte=0,lte=100"`
	Active             *bool       `json:"active,omitempty"`
}

type StoreCustomerRequest struct {
	Document string  `json:"document" binding:"required,min=11,max=18"`
	Name     string  `json:"name" binding:"required,min=2,max=120"`
	Phone    string  `json:"phone" binding:"required,min=10,max=20"`
	Email    *string `json:"email,omitempty" binding:"omitempty,email,max=255"`
	Segment  string  `json:"segment" binding:"omitempty,oneof=varejo atacado construtora"`
}
