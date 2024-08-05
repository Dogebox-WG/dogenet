default: dogenet

.PHONY: clean, test
clean:
	rm -rf ./dogenet

dogenet: clean
	go build -o dogenet ./cmd/dogenet/. 

dev1:
	KEY=$(shell cat dev1-key) go run ./cmd/dogenet --db storage/dev1.db --bind 127.0.0.1:8096 --web 127.0.0.1:8086

dev:
	KEY=$(shell cat dev-key) go run ./cmd/dogenet

key:
	go run ./cmd/dogenet genkey

dev-key:
	go run ./cmd/dogenet genkey >dev-key

dev1-key:
	go run ./cmd/dogenet genkey >dev1-key

test:
	go test -v ./test
