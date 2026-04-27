package handler

import "time"

// forgotPasswordMinElapsed é o tempo mínimo de resposta de ForgotPassword.
// Mascara a diferença entre "email cadastrado" (faz INSERT no DB) e
// "email não cadastrado" (retorna direto), prevenindo enumeração via timing
// (A9-H5). Valor escolhido pra englobar query + INSERT no path lento.
const forgotPasswordMinElapsed = 200 * time.Millisecond

// padToMinElapsed bloqueia até pelo menos `min` ter decorrido desde `start`.
// Usado pra normalizar tempo de respostas sensíveis a timing.
func padToMinElapsed(start time.Time, min time.Duration) {
	if elapsed := time.Since(start); elapsed < min {
		time.Sleep(min - elapsed)
	}
}
