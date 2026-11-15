module github.com/flowcatalyst/go-sdk/tests

go 1.27

require (
	github.com/fergusstrange/embedded-postgres v1.34.0
	github.com/flowcatalyst/go-sdk v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.13.0
	github.com/stretchr/testify v1.12.1
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace github.com/flowcatalyst/go-sdk => ../
