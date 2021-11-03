GOOS=linux
GOARCH=amd64
CGO_ENABLED=0
TARGET=build/sbercloud-dns-acme-helper

all:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o $(TARGET) main.go
