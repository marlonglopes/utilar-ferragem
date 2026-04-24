package model

import "time"

type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	CPF           *string   `json:"cpf,omitempty"`
	Phone         *string   `json:"phone,omitempty"`
	Role          string    `json:"role"`
	EmailVerified bool      `json:"emailVerified"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Address struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	Label        string    `json:"label"`
	Street       string    `json:"street"`
	Number       string    `json:"number"`
	Complement   *string   `json:"complement,omitempty"`
	Neighborhood string    `json:"neighborhood"`
	City         string    `json:"city"`
	State        string    `json:"state"`
	CEP          string    `json:"cep"`
	IsDefault    bool      `json:"isDefault"`
	CreatedAt    time.Time `json:"createdAt"`
}

// -- Request payloads -------------------------------------------------------

type RegisterRequest struct {
	Email    string  `json:"email" binding:"required,email"`
	Password string  `json:"password" binding:"required,min=8,max=72"`
	Name     string  `json:"name" binding:"required,min=2"`
	CPF      *string `json:"cpf,omitempty"`
	Phone    *string `json:"phone,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=72"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type AddressRequest struct {
	Label        string  `json:"label"`
	Street       string  `json:"street" binding:"required"`
	Number       string  `json:"number" binding:"required"`
	Complement   *string `json:"complement,omitempty"`
	Neighborhood string  `json:"neighborhood" binding:"required"`
	City         string  `json:"city" binding:"required"`
	State        string  `json:"state" binding:"required,len=2"`
	CEP          string  `json:"cep" binding:"required"`
	IsDefault    bool    `json:"isDefault"`
}

// -- Response payloads ------------------------------------------------------

type AuthResponse struct {
	User         User   `json:"user"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}
