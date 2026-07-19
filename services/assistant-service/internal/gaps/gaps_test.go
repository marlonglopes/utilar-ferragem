package gaps_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/utilar/assistant-service/internal/gaps"
)

// ---------------------------------------------------------------------------
// Agregação por tema
// ---------------------------------------------------------------------------

// Por que: o registro existe para virar fila de ingestão. Guardar uma linha por
// pergunta não diria o que priorizar — o sinal útil é quantas vezes o MESMO
// buraco apareceu.
func TestRegistrar_AgregaMesmoTemaEmUmaLacuna(t *testing.T) {
	r := gaps.New()

	r.Registrar("consumo de argamassa ac-iii", "sem documento publicado")
	r.Registrar("consumo de argamassa ac-iii", "sem documento publicado")

	top := r.Top(0)
	if len(top) != 1 {
		t.Fatalf("esperado 1 tema agregado, veio %d: %+v", len(top), top)
	}
	if top[0].Ocorrencias != 2 {
		t.Errorf("Ocorrencias = %d, esperado 2", top[0].Ocorrencias)
	}
	if r.Total() != 1 {
		t.Errorf("Total = %d, esperado 1", r.Total())
	}
	if top[0].PrimeiraEm.IsZero() || top[0].UltimaEm.IsZero() {
		t.Error("carimbos de tempo não preenchidos — não daria para saber se a lacuna é atual")
	}
	if top[0].UltimaEm.Before(top[0].PrimeiraEm) {
		t.Error("UltimaEm anterior a PrimeiraEm")
	}
}

// Por que: a mesma dúvida chega escrita de mil jeitos. Sem normalizar, a
// contagem se dilui entre variações e a fila de prioridade fica achatada —
// nada parece urgente.
func TestRegistrar_NormalizaCaixaEEspacosEmBranco(t *testing.T) {
	r := gaps.New()

	variacoes := []string{
		"Quanto de CIMENTO  ",
		"quanto de cimento",
		"  QUANTO   DE cimento ",
		"\tQuanto\nde\tcimento\n",
	}
	for _, v := range variacoes {
		r.Registrar(v, "")
	}

	top := r.Top(0)
	if len(top) != 1 {
		t.Fatalf("variações viraram %d temas, esperado 1: %+v", len(top), top)
	}
	if top[0].Tema != "quanto de cimento" {
		t.Errorf("Tema = %q, esperado a forma normalizada", top[0].Tema)
	}
	if top[0].Ocorrencias != len(variacoes) {
		t.Errorf("Ocorrencias = %d, esperado %d", top[0].Ocorrencias, len(variacoes))
	}
}

// Por que: chamada com tema em branco vem de bug de integração, não de dúvida
// real. Registrar isso só polui a fila com uma linha vazia de alto contador.
func TestRegistrar_IgnoraTemaVazioOuEmBranco(t *testing.T) {
	r := gaps.New()

	for _, vazio := range []string{"", " ", "\t", "\n", "   \t \n "} {
		r.Registrar(vazio, "motivo qualquer")
	}

	if r.Total() != 0 {
		t.Errorf("Total = %d, esperado 0: %+v", r.Total(), r.Top(0))
	}
}

// ---------------------------------------------------------------------------
// LGPD — dado pessoal nunca entra
// ---------------------------------------------------------------------------

// Por que: o endpoint é público e o tema chega de texto do cliente. Um e-mail,
// telefone ou CPF que entre aqui vira base de dados pessoais retida sem
// finalidade — e o valor de uma lacuna individual não paga esse risco.
func TestRegistrar_DescartaTemaComDadoPessoal(t *testing.T) {
	casos := []struct {
		nome string
		tema string
	}{
		{"e-mail", "me manda no joao.silva@exemplo.com o preço"},
		{"arroba solta", "falar com @marlon sobre a obra"},
		{"telefone com DDD", "me liga em 11 98765-4321"},
		{"telefone sem formatação", "contato 11987654321"},
		{"CPF formatado", "cpf 123.456.789-00 para nota fiscal"},
		{"CEP mais número", "entrega no cep 01310-100 numero 500"},
		{"dígitos espalhados", "pedido 1234 nota 5678 obra"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := gaps.New()
			r.Registrar(c.tema, "sem resposta")
			if r.Total() != 0 {
				t.Errorf("dado pessoal foi retido: %+v", r.Top(0))
			}
		})
	}
}

// Por que: um filtro agressivo demais mata o uso legítimo. Quase toda pergunta
// de obra tem número ("parede de 10 m2", "piso 30x30") — se essas sumirem, o
// registro deixa de apontar o que ingerir e a proteção vira sabotagem.
func TestRegistrar_MantemMedidasDeObraLegitimas(t *testing.T) {
	temas := []string{
		"parede de 10 m2",
		"piso 30x30",
		"quantos sacos para 45 m2",
		"argamassa ac-iii consumo por m2",
		"tubo de 100 mm cai bem em esgoto",
		"telha de 2,44 m por 1,10 m",
		"cimento cp-ii 50 kg rende quanto",
	}

	r := gaps.New()
	for _, tema := range temas {
		r.Registrar(tema, "sem documento publicado")
	}

	if r.Total() != len(temas) {
		registrados := map[string]bool{}
		for _, l := range r.Top(0) {
			registrados[l.Tema] = true
		}
		for _, tema := range temas {
			if !registrados[tema] {
				t.Errorf("medida de obra legítima foi descartada pelo filtro de dado pessoal: %q", tema)
			}
		}
		t.Fatalf("Total = %d, esperado %d", r.Total(), len(temas))
	}
}

// Por que: texto longo é onde nome, endereço e histórico se escondem. Cortar em
// maxTamanhoTema limita a superfície do que fica retido.
func TestRegistrar_TruncaTemaMuitoLongo(t *testing.T) {
	const maxTamanhoTema = 120

	longo := strings.TrimSpace(strings.Repeat("argamassa colante para area externa ", 20))
	if len(longo) <= maxTamanhoTema {
		t.Fatalf("tema de teste curto demais (%d) — o teste não exercitaria a truncagem", len(longo))
	}

	r := gaps.New()
	r.Registrar(longo, "")

	top := r.Top(0)
	if len(top) != 1 {
		t.Fatalf("esperado 1 lacuna, veio %d", len(top))
	}
	if len(top[0].Tema) != maxTamanhoTema {
		t.Errorf("tamanho do tema = %d, esperado %d", len(top[0].Tema), maxTamanhoTema)
	}
	if !strings.HasPrefix(longo, top[0].Tema) {
		t.Errorf("truncagem não preservou o prefixo: %q", top[0].Tema)
	}
}

// Por que: dois temas longos que só diferem depois do corte colapsam no mesmo
// registro. É o preço aceito da truncagem — o teste registra a consequência
// para que ninguém a descubra como surpresa em produção.
func TestRegistrar_TemasLongosComPrefixoIgualColapsam(t *testing.T) {
	base := strings.TrimSpace(strings.Repeat("argamassa colante area externa ", 6))
	r := gaps.New()

	r.Registrar(base+" fachada", "")
	r.Registrar(base+" piscina", "")

	if r.Total() != 1 {
		t.Errorf("Total = %d — a truncagem deveria colapsar os dois em 1: %+v", r.Total(), r.Top(0))
	}
}

// ---------------------------------------------------------------------------
// Teto de memória
// ---------------------------------------------------------------------------

// Por que: o endpoint que alimenta este registro é público. Sem teto, mandar
// perguntas aleatórias em volume infla o mapa até derrubar o serviço — o
// registro de lacunas viraria o vetor de negação de serviço.
func TestRegistrar_RespeitaTetoDeTemas(t *testing.T) {
	r := gaps.New()

	for i := 0; i < gaps.MaxTemas; i++ {
		r.Registrar(fmt.Sprintf("tema numero %s", porExtenso(i)), "")
	}
	if r.Total() != gaps.MaxTemas {
		t.Fatalf("Total = %d, esperado %d antes do teto", r.Total(), gaps.MaxTemas)
	}

	for i := 0; i < 50; i++ {
		r.Registrar(fmt.Sprintf("tema extra %s", porExtenso(i)), "")
	}
	if r.Total() != gaps.MaxTemas {
		t.Errorf("Total = %d — o registro cresceu depois do teto", r.Total())
	}

	// Contar ocorrência de tema JÁ conhecido continua funcionando: o teto limita
	// temas distintos, não a contagem de um buraco real que segue aparecendo.
	antes := ocorrenciasDe(t, r, "tema numero zero")
	r.Registrar("Tema Numero ZERO", "")
	if depois := ocorrenciasDe(t, r, "tema numero zero"); depois != antes+1 {
		t.Errorf("ocorrências de tema existente = %d, esperado %d — o teto está bloqueando contagem legítima", depois, antes+1)
	}
}

// ---------------------------------------------------------------------------
// Top — a fila de prioridade de ingestão
// ---------------------------------------------------------------------------

// Por que: a ordem É o produto deste pacote. Quem lê Top espera "o que ingerir
// primeiro", e o desempate precisa ser estável para o relatório não dançar
// entre duas execuções idênticas.
func TestTop_OrdenaPorOcorrenciasComDesempateEstavel(t *testing.T) {
	r := gaps.New()

	repetir := func(tema string, n int) {
		for i := 0; i < n; i++ {
			r.Registrar(tema, "")
		}
	}
	repetir("impermeabilizante de laje", 5)
	repetir("argamassa ac-iii", 2)
	repetir("betoneira aluguel", 2)
	repetir("areia media", 1)

	quer := []string{"impermeabilizante de laje", "argamassa ac-iii", "betoneira aluguel", "areia media"}

	// Roda várias vezes: ordem instável (iteração de mapa) apareceria aqui.
	for tentativa := 0; tentativa < 5; tentativa++ {
		got := r.Top(0)
		if len(got) != len(quer) {
			t.Fatalf("Top(0) devolveu %d, esperado %d", len(got), len(quer))
		}
		for i, tema := range quer {
			if got[i].Tema != tema {
				t.Fatalf("tentativa %d: Top(0)[%d].Tema = %q, esperado %q", tentativa, i, got[i].Tema, tema)
			}
		}
	}

	t.Run("n limita o resultado", func(t *testing.T) {
		if got := r.Top(2); len(got) != 2 || got[0].Tema != "impermeabilizante de laje" {
			t.Errorf("Top(2) = %+v", got)
		}
	})

	t.Run("n maior que o total devolve tudo", func(t *testing.T) {
		if got := r.Top(99); len(got) != len(quer) {
			t.Errorf("Top(99) devolveu %d, esperado %d", len(got), len(quer))
		}
	})

	t.Run("n zero ou negativo devolve tudo", func(t *testing.T) {
		for _, n := range []int{0, -1} {
			if got := r.Top(n); len(got) != len(quer) {
				t.Errorf("Top(%d) devolveu %d, esperado %d", n, len(got), len(quer))
			}
		}
	})
}

// Por que: Top devolve cópias. Se devolvesse ponteiros, um handler HTTP
// serializando o relatório poderia alterar o contador real do registro.
func TestTop_DevolveCopiasEnaoOEstadoInterno(t *testing.T) {
	r := gaps.New()
	r.Registrar("argamassa ac-iii", "sem documento")

	got := r.Top(0)
	got[0].Ocorrencias = 999
	got[0].Tema = "adulterado"

	depois := r.Top(0)
	if depois[0].Ocorrencias != 1 || depois[0].Tema != "argamassa ac-iii" {
		t.Errorf("estado interno mutado por fora: %+v", depois[0])
	}
}

// Por que: o motivo é texto de diagnóstico que também pode carregar dado do
// cliente. Normalizá-lo junto com o tema mantém o registro previsível.
func TestRegistrar_NormalizaMotivoEMantemODaPrimeiraOcorrencia(t *testing.T) {
	r := gaps.New()

	r.Registrar("argamassa ac-iii", "  SEM Documento   Publicado ")
	r.Registrar("argamassa ac-iii", "outro motivo qualquer")

	top := r.Top(0)
	if len(top) != 1 {
		t.Fatalf("esperado 1 lacuna, veio %d", len(top))
	}
	if top[0].Motivo != "sem documento publicado" {
		t.Errorf("Motivo = %q, esperado a forma normalizada da primeira ocorrência", top[0].Motivo)
	}
}

// Por que: registro recém-criado precisa ser seguro de ler. Um handler que
// chama Top antes da primeira pergunta não pode receber nil-panic.
func TestTop_RegistroVazioDevolveListaVazia(t *testing.T) {
	r := gaps.New()

	if got := r.Top(0); len(got) != 0 {
		t.Errorf("Top(0) em registro vazio = %+v", got)
	}
	if r.Total() != 0 {
		t.Errorf("Total = %d em registro vazio", r.Total())
	}
}

// ---------------------------------------------------------------------------
// Concorrência
// ---------------------------------------------------------------------------

// Por que: o registro é global e escrito por toda requisição que a Alice não
// soube responder, enquanto o painel de curadoria lê Top em paralelo. Corrida
// aqui corrompe o mapa que decide o que a equipe vai ingerir.
func TestRegistro_ConcorrenciaRegistrarTopTotal(t *testing.T) {
	r := gaps.New()

	const goroutines = 16
	const iteracoes = 60

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iteracoes; i++ {
				switch g % 4 {
				case 0:
					r.Registrar("argamassa ac-iii", "sem documento publicado")
				case 1:
					r.Registrar(fmt.Sprintf("tema %s", porExtenso(i)), "sem documento")
				case 2:
					_ = r.Top(5)
				case 3:
					_ = r.Total()
					r.Registrar("joao@exemplo.com quer orcamento", "descartar")
				}
			}
		}(g)
	}
	wg.Wait()

	quer := goroutines / 4 * iteracoes
	if got := ocorrenciasDe(t, r, "argamassa ac-iii"); got != quer {
		t.Errorf("ocorrências sob concorrência = %d, esperado %d (contador perdeu incrementos)", got, quer)
	}
	for _, l := range r.Top(0) {
		if strings.Contains(l.Tema, "@") {
			t.Errorf("dado pessoal entrou sob concorrência: %q", l.Tema)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ocorrenciasDe(t *testing.T, r *gaps.Registro, tema string) int {
	t.Helper()
	for _, l := range r.Top(0) {
		if l.Tema == tema {
			return l.Ocorrencias
		}
	}
	return 0
}

// porExtenso gera temas distintos SEM dígitos, para não esbarrar no filtro de
// dado pessoal (8+ dígitos) e acabar testando outra coisa por acidente.
func porExtenso(n int) string {
	nomes := []string{"zero", "um", "dois", "tres", "quatro", "cinco", "seis", "sete", "oito", "nove"}
	if n == 0 {
		return nomes[0]
	}
	var partes []string
	for n > 0 {
		partes = append([]string{nomes[n%10]}, partes...)
		n /= 10
	}
	return strings.Join(partes, "-")
}
