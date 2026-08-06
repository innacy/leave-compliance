tidy:
	go mod tidy

build-windows:
	GOOS=windows GOARCH=amd64 go build -o attendance.exe