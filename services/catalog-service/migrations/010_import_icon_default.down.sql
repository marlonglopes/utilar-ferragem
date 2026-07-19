-- Reverte 010. Só remove o DEFAULT — os valores já gravados permanecem, porque
-- removê-los reintroduziria o NULL que o NOT NULL proíbe.
ALTER TABLE products ALTER COLUMN icon DROP DEFAULT;
