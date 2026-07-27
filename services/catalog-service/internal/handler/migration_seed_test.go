package handler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regressão: "catalog-db-reset morria no migrate".
//
// A migration 016 trazia o INSERT das regras de complemento (product_complement_rules).
// Cada regra referencia categories(id) por FK, mas as categorias são dado de SEED
// (migrations/seed.sql), aplicado DEPOIS do migrate. Num `migrate` limpo a categoria
// ainda não existe, então o INSERT estourava:
//
//	insert or update on table "product_complement_rules" violates foreign key
//	constraint ... Key (source_category_id)=(construcao) is not present in
//	table "categories".
//
// Isso quebrava o reset inteiro (migrate-down; migrate; seed → morria no migrate),
// derrubando toda a suíte de integração do catalog. Regra geral: SCHEMA vai na
// migration; DADO que depende de seed vai no seed.

const migrationsDir = "../../migrations"

// Guarda estática (roda sempre, sem DB): nenhuma migration .up.sql pode conter
// INSERT de dado que depende de categorias de seed. Se alguém reintroduzir esse
// INSERT numa migration, o teste falha antes de o reset quebrar em runtime.
func TestRegression_MigrationsNaoInseremDadoDependenteDeCategoriaSeed(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("não consegui ler %s: %v", migrationsDir, err)
	}
	// Tabelas cujo dado semente referencia categories(id) por FK e, portanto,
	// só pode ser inserido no seed (depois das categorias), nunca numa migration.
	seedOnlyTables := []string{"product_complement_rules"}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("lendo %s: %v", name, err)
		}
		lower := strings.ToLower(string(body))
		for _, tbl := range seedOnlyTables {
			if strings.Contains(lower, "insert into "+tbl) {
				t.Errorf("migration %s faz INSERT em %s — esse dado referencia "+
					"categories(id) (seed) e quebra o migrate limpo. Mova o INSERT "+
					"para migrations/seed.sql.", name, tbl)
			}
		}
	}
}

// Guarda viva (skipa sem DB): no banco semeado, as regras carregaram e TODO FK
// (source/target category) resolve. Prova que o seed roda sem violar a FK.
func TestRegression_SeedComplementRules_CarregaSemViolarFK(t *testing.T) {
	db := setupTestDB(t) // skipa se DB indisponível ou sem seed

	var total int
	if err := db.QueryRow(`SELECT count(*) FROM product_complement_rules`).Scan(&total); err != nil {
		t.Skipf("product_complement_rules ainda não migrada: %v", err)
	}
	if total == 0 {
		t.Fatal("nenhuma regra de complemento no seed — o INSERT sumiu do seed.sql")
	}

	// Qualquer FK órfã (categoria referenciada inexistente) significa que o seed
	// carregou dado inconsistente — o exato modo de falha que derrubava o migrate.
	var orfas int
	err := db.QueryRow(`
		SELECT count(*) FROM product_complement_rules r
		WHERE (r.source_category_id IS NOT NULL
		       AND NOT EXISTS (SELECT 1 FROM categories c WHERE c.id = r.source_category_id))
		   OR (r.target_category_id IS NOT NULL
		       AND NOT EXISTS (SELECT 1 FROM categories c WHERE c.id = r.target_category_id))
	`).Scan(&orfas)
	if err != nil {
		t.Fatalf("checando FKs de complemento: %v", err)
	}
	if orfas != 0 {
		t.Errorf("%d regra(s) de complemento com categoria inexistente — FK órfã no seed", orfas)
	}
}
