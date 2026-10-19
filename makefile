tidy:
	go mod tidy

build-sql-fetcher:
	GOOS=windows GOARCH=amd64 go build -o fetchFromSQL.exe ./cmd/fetchFromSQL

build-machine-fetcher:
	GOOS=windows GOARCH=amd64 go build -o fetchFromMachine.exe ./cmd/fetchFromMachine