# Git hooks

Hooks locais que aceleram feedback e bloqueiam commits quebrados.

## Como ativar

```bash
git config core.hooksPath .githooks
```

A partir daí, todo `git commit` roda os hooks deste diretório.

## Hooks disponíveis

### `pre-commit`

Roda `go vet` + `go build` nos módulos cujos arquivos Go foram modificados
(staged). Pula testes — esses rodam em CI. Aborta o commit se vet/build falha.

Não toca em arquivos não-Go.
