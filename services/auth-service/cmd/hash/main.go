// Ferramenta CLI para gerar argon2id hash de uma senha.
// Uso: go run ./cmd/hash <senha>
// Usada para gerar o hash embutido em seed.sql.
package main

import (
	"fmt"
	"os"

	"github.com/utilar/auth-service/internal/auth"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/hash <password>")
		os.Exit(1)
	}
	h, err := auth.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(h)
}
